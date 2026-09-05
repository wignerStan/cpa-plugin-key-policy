package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	modelCatalogSourceOpenAI = "openai-models"
	modelCatalogSourceCodex  = "codex-models"
)

// CatalogGroup is a reusable downstream model catalog assigned to one or more
// key IDs. Key entries support exact IDs, shell-style globs, and "*".
type CatalogGroup struct {
	Name   string         `yaml:"name" json:"name"`
	Keys   []string       `yaml:"keys" json:"keys"`
	Models []CatalogModel `yaml:"models" json:"models"`
}

// CatalogModel clones one source entry from CPA's generated model catalog,
// exposes it under ID, and then applies Patch. Source defaults to ID.
// Remove deletes top-level fields after the patch is merged.
type CatalogModel struct {
	ID     string         `yaml:"id" json:"id"`
	Source string         `yaml:"source,omitempty" json:"source,omitempty"`
	Patch  map[string]any `yaml:"patch,omitempty" json:"patch,omitempty"`
	Remove []string       `yaml:"remove,omitempty" json:"remove,omitempty"`
}

type catalogPolicy struct {
	Groups []CatalogGroup
}

var catalogPolicies sync.Map // map[*App]catalogPolicy

func (a *App) setCatalogPolicy(policy catalogPolicy) {
	if a == nil {
		return
	}
	catalogPolicies.Store(a, policy)
}

func (a *App) catalogPolicySnapshot() catalogPolicy {
	if a == nil {
		return catalogPolicy{}
	}
	if raw, ok := catalogPolicies.Load(a); ok {
		if policy, ok := raw.(catalogPolicy); ok {
			return policy
		}
	}
	return catalogPolicy{}
}

type catalogConfigDocument struct {
	CatalogGroups []CatalogGroup `yaml:"catalog_groups"`
}

func decodeCatalogPolicy(raw []byte) (catalogPolicy, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return catalogPolicy{}, nil
	}
	var doc catalogConfigDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return catalogPolicy{}, err
	}

	seenGroups := make(map[string]struct{}, len(doc.CatalogGroups))
	for gi := range doc.CatalogGroups {
		group := &doc.CatalogGroups[gi]
		group.Name = strings.TrimSpace(group.Name)
		if group.Name == "" {
			return catalogPolicy{}, fmt.Errorf("catalog_groups[%d]: name is required", gi)
		}
		groupKey := strings.ToLower(group.Name)
		if _, exists := seenGroups[groupKey]; exists {
			return catalogPolicy{}, fmt.Errorf("duplicate catalog group %q", group.Name)
		}
		seenGroups[groupKey] = struct{}{}

		keys := make([]string, 0, len(group.Keys))
		seenKeys := make(map[string]struct{}, len(group.Keys))
		for _, rawKey := range group.Keys {
			key := strings.TrimSpace(rawKey)
			if key == "" {
				continue
			}
			if _, err := path.Match(key, "catalog-key-validation"); err != nil {
				return catalogPolicy{}, fmt.Errorf("catalog group %q key pattern %q: %w", group.Name, key, err)
			}
			lk := strings.ToLower(key)
			if _, exists := seenKeys[lk]; exists {
				continue
			}
			seenKeys[lk] = struct{}{}
			keys = append(keys, key)
		}
		group.Keys = keys

		seenModels := make(map[string]struct{}, len(group.Models))
		for mi := range group.Models {
			model := &group.Models[mi]
			model.ID = strings.TrimSpace(model.ID)
			model.Source = strings.TrimSpace(model.Source)
			if model.ID == "" {
				return catalogPolicy{}, fmt.Errorf("catalog group %q model %d: id is required", group.Name, mi)
			}
			if model.Source == "" {
				model.Source = model.ID
			}
			lm := strings.ToLower(model.ID)
			if _, exists := seenModels[lm]; exists {
				return catalogPolicy{}, fmt.Errorf("catalog group %q has duplicate model id %q", group.Name, model.ID)
			}
			seenModels[lm] = struct{}{}
			cleanRemove := make([]string, 0, len(model.Remove))
			seenRemove := make(map[string]struct{}, len(model.Remove))
			for _, rawField := range model.Remove {
				field := strings.TrimSpace(rawField)
				if field == "" {
					continue
				}
				if _, exists := seenRemove[field]; exists {
					continue
				}
				seenRemove[field] = struct{}{}
				cleanRemove = append(cleanRemove, field)
			}
			model.Remove = cleanRemove
		}
	}
	return catalogPolicy{Groups: doc.CatalogGroups}, nil
}

func isModelCatalogResponse(req ResponseInterceptRequest) bool {
	switch strings.ToLower(strings.TrimSpace(req.SourceFormat)) {
	case modelCatalogSourceOpenAI, modelCatalogSourceCodex:
		return true
	}
	if req.Metadata != nil {
		if rawPath, ok := req.Metadata["request_path"]; ok && strings.TrimSpace(fmt.Sprint(rawPath)) == "/v1/models" {
			return true
		}
	}
	return false
}

func (a *App) rewriteModelCatalog(req ResponseInterceptRequest) ([]byte, bool) {
	if a == nil {
		return nil, false
	}
	policy := a.catalogPolicySnapshot()
	if len(policy.Groups) == 0 {
		return nil, false
	}

	keyID := catalogKeyID(req.Metadata)
	if keyID == "" {
		// Native CPA keys and other frontend auth providers are not governed by
		// this plugin's catalog groups.
		return nil, false
	}

	models := catalogModelsForKey(policy.Groups, keyID)
	body, err := rewriteCatalogBody(req.Body, models)
	if err != nil {
		// Catalog groups are an allow-list. If a patched host ever changes the
		// response shape unexpectedly, fail closed to an empty valid catalog
		// rather than leaking the global list to a restricted plugin key.
		return emptyCatalogBody(req), true
	}
	return body, !bytes.Equal(body, req.Body)
}

func catalogKeyID(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	// The CPA model-list middleware stamps the frontend auth provider. Do not
	// consume another auth plugin's metadata merely because it also has key_id.
	if rawProvider, exists := metadata["access_provider"]; exists {
		provider := strings.TrimSpace(fmt.Sprint(rawProvider))
		if provider != "" && !strings.EqualFold(provider, PluginID) {
			return ""
		}
	}
	if direct, ok := metadata["key_id"]; ok {
		if key := strings.TrimSpace(fmt.Sprint(direct)); key != "" {
			return key
		}
	}
	raw, ok := metadata["access_metadata"]
	if !ok || raw == nil {
		return ""
	}
	switch meta := raw.(type) {
	case map[string]any:
		if value, ok := meta["key_id"]; ok {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	case map[string]string:
		return strings.TrimSpace(meta["key_id"])
	}
	return ""
}

func catalogModelsForKey(groups []CatalogGroup, keyID string) []CatalogModel {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]CatalogModel, 0)
	for _, group := range groups {
		if !catalogGroupMatchesKey(group, keyID) {
			continue
		}
		for _, model := range group.Models {
			lm := strings.ToLower(model.ID)
			if _, exists := seen[lm]; exists {
				continue
			}
			seen[lm] = struct{}{}
			out = append(out, model)
		}
	}
	return out
}

func catalogGroupMatchesKey(group CatalogGroup, keyID string) bool {
	for _, pattern := range group.Keys {
		matched, err := path.Match(pattern, keyID)
		if err == nil && matched {
			return true
		}
		// Exact key IDs should remain case-insensitive to match policy key lookup.
		if !strings.ContainsAny(pattern, "*?[") && strings.EqualFold(pattern, keyID) {
			return true
		}
	}
	return false
}

func rewriteCatalogBody(body []byte, requested []CatalogModel) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	if raw, ok := root["models"]; ok {
		models, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("models catalog field is not an array")
		}
		root["models"] = selectCatalogModels(models, requested, true)
		return json.Marshal(root)
	}
	if raw, ok := root["data"]; ok {
		models, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("data catalog field is not an array")
		}
		root["data"] = selectCatalogModels(models, requested, false)
		return json.Marshal(root)
	}
	return nil, fmt.Errorf("unrecognized model catalog shape")
}

func emptyCatalogBody(req ResponseInterceptRequest) []byte {
	if strings.EqualFold(strings.TrimSpace(req.SourceFormat), modelCatalogSourceCodex) {
		return []byte(`{"models":[]}`)
	}
	if req.Metadata != nil {
		if _, exists := req.Metadata["client_version"]; exists {
			return []byte(`{"models":[]}`)
		}
	}
	return []byte(`{"object":"list","data":[]}`)
}

func selectCatalogModels(source []any, requested []CatalogModel, codex bool) []any {
	byID := make(map[string]map[string]any, len(source))
	for _, raw := range source {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for _, id := range catalogEntryIDs(entry, codex) {
			if id == "" {
				continue
			}
			key := strings.ToLower(id)
			if _, exists := byID[key]; !exists {
				byID[key] = entry
			}
		}
	}

	out := make([]any, 0, len(requested))
	for _, spec := range requested {
		sourceEntry := byID[strings.ToLower(spec.Source)]
		if sourceEntry == nil {
			continue
		}
		entry := cloneJSONMap(sourceEntry)
		mergeJSONMap(entry, spec.Patch)
		for _, field := range spec.Remove {
			delete(entry, field)
		}
		if codex {
			entry["slug"] = spec.ID
			if _, exists := entry["id"]; exists {
				entry["id"] = spec.ID
			}
		} else {
			entry["id"] = spec.ID
		}
		out = append(out, entry)
	}
	return out
}

func catalogEntryIDs(entry map[string]any, codex bool) []string {
	ids := make([]string, 0, 2)
	if codex {
		if value, ok := entry["slug"].(string); ok {
			ids = append(ids, strings.TrimSpace(value))
		}
	}
	if value, ok := entry["id"].(string); ok {
		ids = append(ids, strings.TrimSpace(value))
	}
	return ids
}

func cloneJSONMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = cloneJSONValue(value)
	}
	return out
}

func cloneJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneJSONMap(v)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = cloneJSONValue(v[i])
		}
		return out
	default:
		return v
	}
}

func mergeJSONMap(dst map[string]any, patch map[string]any) {
	for key, patchValue := range patch {
		patchMap, patchIsMap := patchValue.(map[string]any)
		if !patchIsMap {
			dst[key] = cloneJSONValue(patchValue)
			continue
		}
		currentMap, currentIsMap := dst[key].(map[string]any)
		if !currentIsMap {
			currentMap = make(map[string]any)
		}
		mergeJSONMap(currentMap, patchMap)
		dst[key] = currentMap
	}
}
