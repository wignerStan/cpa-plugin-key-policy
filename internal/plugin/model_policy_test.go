package plugin

import (
	"encoding/json"
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
