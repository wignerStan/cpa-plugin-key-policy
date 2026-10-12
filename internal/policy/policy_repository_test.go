package policy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONPolicyIsAuthoritativeOnReload(t *testing.T) {
	store, secret := newTestStore(t)
	path := store.StatePath()
	// Host settings must not replace policy, including key permissions and scheduling.
	if err := store.Configure(Config{Enabled: false, StateFile: path, GlobalWeightedRoundRobin: true}); err != nil {
		t.Fatal(err)
	}
	if store.FindByAPIKey(secret) == nil || !store.Enabled() || store.GlobalWeightedRoundRobin() {
		t.Fatal("host configuration overrode JSON")
	}
	state, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	state.Keys = nil
	raw, _ := json.Marshal(state)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = store.ReloadPolicy(nil); err != nil {
		t.Fatal(err)
	}
	if store.FindByAPIKey(secret) != nil {
		t.Fatal("JSON revocation did not invalidate memory cache")
	}
}
func TestPolicyWriteFailureDoesNotPublishToCache(t *testing.T) {
	store, _ := newTestStore(t)
	path := store.StatePath()
	before := store.Keys()
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	h, _ := HashKey("new-key")
	if err := store.UpsertKey(KeyConfig{ID: "new", Enabled: true, KeyHash: h, IncludeModels: []string{"gpt-*"}}); err == nil {
		t.Fatal("expected failed JSON write")
	}
	if len(store.Keys()) != len(before) || store.FindByAPIKey("new-key") != nil {
		t.Fatal("uncommitted policy entered memory cache")
	}
}
func TestPolicyNotificationRunsAfterCommit(t *testing.T) {
	store, _ := newTestStore(t)
	notified := false
	store.SetOnClassifyRulesChanged(func() {
		state, err := LoadState(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		if state.GlobalWeightedRoundRobin == nil || !*state.GlobalWeightedRoundRobin {
			t.Fatal("notified before JSON commit")
		}
		if !store.GlobalWeightedRoundRobin() {
			t.Fatal("notification observed stale cache")
		}
		notified = true
	})
	if err := store.SetGlobalWeightedRoundRobin(true); err != nil {
		t.Fatal(err)
	}
	if !notified {
		t.Fatal("no cache notification")
	}
}
func TestUsageFlushPreservesCatalogAndPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	state := State{Version: 3, CatalogGroups: json.RawMessage(`[{"name":"gpt","keys":["k"],"include_models":["gpt-*"]}]`), Keys: []KeyConfig{{ID: "k", Enabled: true}}}
	raw, _ := json.Marshal(state)
	os.WriteFile(path, raw, 0600)
	if err := SaveUsageOnly(path, map[string]*UsageState{"k": {Daily: UsageWindow{TotalUSD: 12}}}); err != nil {
		t.Fatal(err)
	}
	after, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	var got, want bytes.Buffer
	json.Compact(&got, after.CatalogGroups)
	json.Compact(&want, state.CatalogGroups)
	if len(after.Keys) != 1 || got.String() != want.String() || after.Usage["k"].Daily.TotalUSD != 12 {
		t.Fatal("usage flush changed policy")
	}
}
func TestJSONKeySelectorsSurviveRestart(t *testing.T) {
	store, _ := newTestStore(t)
	h, _ := HashKey("ustc-test")
	if err := store.UpsertKey(KeyConfig{ID: "ustc", Enabled: true, KeyHash: h, IncludeModels: []string{"ustc/*"}, AllowModelsEndpoint: true}); err != nil {
		t.Fatal(err)
	}
	restarted := NewStore()
	if err := restarted.Configure(Config{StateFile: store.StatePath()}); err != nil {
		t.Fatal(err)
	}
	d := restarted.Authenticate("POST", "/v1/responses", http.Header{"Authorization": []string{"Bearer ustc-test"}}, nil, []byte(`{"model":"ustc/deepseek-flash"}`))
	if !d.Allowed {
		t.Fatalf("USTC policy lost on restart: %+v", d)
	}
}

func TestJSONPlaintextKeyIsHashedBeforeCachePublication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	raw := []byte(`{"version":3,"keys":[{"id":"plain","enabled":true,"key":"one-time-json-secret","include_models":["ustc/*"]}]}`)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	if err := store.Configure(Config{StateFile: path}); err != nil {
		t.Fatal(err)
	}
	if store.FindByAPIKey("one-time-json-secret") == nil {
		t.Fatal("imported key not authenticatable")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(after, []byte("one-time-json-secret")) || bytes.Contains(after, []byte(`"key":`)) {
		t.Fatal("plaintext JSON key was not scrubbed")
	}
}
func TestPatchUsesLatestJSONInsteadOfStaleMemory(t *testing.T) {
	store, _ := newTestStore(t)
	state, err := LoadState(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	state.Keys[0].DailyLimitUSD = 25
	raw, _ := json.Marshal(state)
	if err = os.WriteFile(store.StatePath(), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = store.PatchKey(state.Keys[0].ID, func(k *KeyConfig) error { k.Name = "Updated"; return nil }); err != nil {
		t.Fatal(err)
	}
	after, err := LoadState(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if after.Keys[0].DailyLimitUSD != 25 || after.Keys[0].Name != "Updated" {
		t.Fatal("patch overwrote authoritative policy from a stale cache")
	}
}
