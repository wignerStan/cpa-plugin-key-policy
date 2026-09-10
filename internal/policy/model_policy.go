package policy

import (
	"fmt"
	"strings"
)

// normalizeModelPatterns trims, removes empty entries, and de-duplicates
// selectors case-insensitively while preserving their first spelling/order.
// A selector is either an exact model/alias name or a single trailing "*"
// prefix wildcard. Keeping the grammar intentionally small avoids surprising
// glob/regex behavior in an authorization rule.
func normalizeModelPatterns(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if strings.Count(value, "*") > 1 || (strings.Contains(value, "*") && !strings.HasSuffix(value, "*")) {
			return nil, fmt.Errorf("model selector %q must be exact or use one trailing *", value)
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}

func modelPatternMatches(model, pattern string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if model == "" || pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(model, strings.TrimSuffix(pattern, "*"))
	}
	return model == pattern
}

func modelMatchesAny(model string, patterns []string) bool {
	for _, pattern := range patterns {
		if modelPatternMatches(model, pattern) {
			return true
		}
	}
	return false
}

// ModelIncluded reports whether a non-explicit/native model is admitted by the
// key's include selectors. Empty include_models means no implicit native-model
// access, preserving the legacy explicit-alias-only behavior.
func (k *KeyConfig) ModelIncluded(model string) bool {
	return k != nil && modelMatchesAny(model, k.IncludeModels)
}

// ModelExcluded reports whether a client-facing alias or resolved target model
// is denied. Exclusions override both explicit mappings and include selectors.
func (k *KeyConfig) ModelExcluded(model string) bool {
	return k != nil && modelMatchesAny(model, k.ExcludeModels)
}
