package plugin

import (
	"cpa-key-policy/internal/policy"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitForPolicy(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("policy notification did not refresh cache")
		case <-tick.C:
		}
	}
}
func TestJSONFileNotificationRefreshesPolicyAndCatalog(t *testing.T) {
	app := configureSettingsApp(t, filepath.Join(t.TempDir(), "policy.json"))
	path := app.store.StatePath()
	state, err := policy.LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := policy.HashKey("test-notified-key")
	state.Keys = []policy.KeyConfig{{ID: "notified", Enabled: true, KeyHash: hash, IncludeModels: []string{"ustc/*"}}}
	state.CatalogGroups = json.RawMessage(`[{"name":"ustc","keys":["notified"],"include_models":["ustc/*"]}]`)
	raw, _ := json.Marshal(state)
	tmp := path + ".new"
	if err = os.WriteFile(tmp, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	waitForPolicy(t, func() bool {
		return app.store.FindByAPIKey("test-notified-key") != nil && len(app.catalogPolicySnapshot().Groups) == 1
	})
	state.Keys = nil
	state.CatalogGroups = json.RawMessage(`[]`)
	raw, _ = json.Marshal(state)
	os.WriteFile(tmp, raw, 0600)
	os.Rename(tmp, path)
	waitForPolicy(t, func() bool {
		return app.store.FindByAPIKey("test-notified-key") == nil && len(app.catalogPolicySnapshot().Groups) == 0
	})
}
func TestInvalidJSONKeepsLastValidPolicy(t *testing.T) {
	app := configureSettingsApp(t, filepath.Join(t.TempDir(), "policy.json"))
	path := app.store.StatePath()
	hash, _ := policy.HashKey("valid-key")
	if err := app.store.UpsertKey(policy.KeyConfig{ID: "valid", Enabled: true, KeyHash: hash}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"keys":`), 0600); err != nil {
		t.Fatal(err)
	}
	waitForPolicy(t, func() bool { return app.policyStatus()["policy_reload_error"] != "" })
	if app.store.FindByAPIKey("valid-key") == nil {
		t.Fatal("invalid document replaced valid cache")
	}
	if err := app.reloadJSONPolicy(); err == nil {
		t.Fatal("invalid explicit reload succeeded")
	}
}
func TestRegistrationRejectsPluginPolicyInYAML(t *testing.T) {
	for _, raw := range []string{"keys: []", "catalog_groups: []", "global_weighted_round_robin: true", "classify_rules: []"} {
		if _, err := decodeRegistration([]byte(raw)); err == nil {
			t.Fatalf("accepted plugin setting in YAML: %s", raw)
		}
	}
}
