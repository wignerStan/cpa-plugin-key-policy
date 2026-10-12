package plugin

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
)

type modelPolicyPublicKeyForTest struct {
	ID            string   `json:"id"`
	IncludeModels []string `json:"include_models"`
	ExcludeModels []string `json:"exclude_models"`
}

func keyModelPolicyForTest(t *testing.T, app *App, id string) ([]string, []string) {
	t.Helper()
	for _, key := range app.store.Keys() {
		if key.ID == id {
			return key.IncludeModels, key.ExcludeModels
		}
	}
	t.Fatalf("key %q not found", id)
	return nil, nil
}

func requireModelPolicyForTest(t *testing.T, app *App, id string, include, exclude []string) {
	t.Helper()
	gotInclude, gotExclude := keyModelPolicyForTest(t, app, id)
	if !slices.Equal(gotInclude, include) || !slices.Equal(gotExclude, exclude) {
		t.Fatalf("model policy = include %v exclude %v, want include %v exclude %v", gotInclude, gotExclude, include, exclude)
	}
}

func decodeModelPolicyResponseForTest(t *testing.T, response ManagementResponse, expectedStatus int) modelPolicyPublicKeyForTest {
	t.Helper()
	if response.StatusCode != expectedStatus {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, expectedStatus, response.Body)
	}
	var envelope struct {
		Key modelPolicyPublicKeyForTest `json:"key"`
	}
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, response.Body)
	}
	if envelope.Key.IncludeModels == nil || envelope.Key.ExcludeModels == nil {
		t.Fatalf("model policy arrays must serialize as [], not null: %+v", envelope.Key)
	}
	return envelope.Key
}

func TestKeyModelPolicyPatchFieldsIndependentlyAndPersist(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	app := configureSettingsApp(t, statePath)

	created := callManagementForTest(t, app, http.MethodPost, "/v0/management/plugins/cpa-key-policy/keys", []byte(`{
		"id":"model-policy",
		"key":"cpa_model_policy_patch",
		"include_models":[" gpt-5-* ","Gpt-5-*","claude-sonnet-4",""],
		"exclude_models":["gpt-5-codex-*"]
	}`))
	createdKey := decodeModelPolicyResponseForTest(t, created, http.StatusCreated)
	if !slices.Equal(createdKey.IncludeModels, []string{"gpt-5-*", "claude-sonnet-4"}) ||
		!slices.Equal(createdKey.ExcludeModels, []string{"gpt-5-codex-*"}) {
		t.Fatalf("created key policy = %+v", createdKey)
	}

	includePatch := callManagementForTest(t, app, http.MethodPatch, "/v0/management/plugins/cpa-key-policy/keys", []byte(`{
		"id":"model-policy",
		"include_models":["gemini-2.5-*"]
	}`))
	decodeModelPolicyResponseForTest(t, includePatch, http.StatusOK)
	requireModelPolicyForTest(t, app, "model-policy", []string{"gemini-2.5-*"}, []string{"gpt-5-codex-*"})

	nameOnlyPatch := callManagementForTest(t, app, http.MethodPatch, "/v0/management/plugins/cpa-key-policy/keys", []byte(`{
		"id":"model-policy",
		"name":"renamed"
	}`))
	decodeModelPolicyResponseForTest(t, nameOnlyPatch, http.StatusOK)
	requireModelPolicyForTest(t, app, "model-policy", []string{"gemini-2.5-*"}, []string{"gpt-5-codex-*"})

	clearExclude := callManagementForTest(t, app, http.MethodPatch, "/v0/management/plugins/cpa-key-policy/keys", []byte(`{
		"id":"model-policy",
		"exclude_models":[]
	}`))
	cleared := decodeModelPolicyResponseForTest(t, clearExclude, http.StatusOK)
	if !slices.Equal(cleared.IncludeModels, []string{"gemini-2.5-*"}) || len(cleared.ExcludeModels) != 0 {
		t.Fatalf("clear response = %+v", cleared)
	}

	restarted := configureSettingsApp(t, statePath)
	requireModelPolicyForTest(t, restarted, "model-policy", []string{"gemini-2.5-*"}, []string{})
}

func TestKeyModelPolicyRejectsNonPrefixWildcard(t *testing.T) {
	app := configureSettingsApp(t, filepath.Join(t.TempDir(), "state.json"))
	response := callManagementForTest(t, app, http.MethodPost, "/v0/management/plugins/cpa-key-policy/keys", []byte(`{
		"id":"bad-pattern",
		"key":"cpa_bad_pattern",
		"include_models":["gpt-*mini"]
	}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.StatusCode, response.Body)
	}
}
