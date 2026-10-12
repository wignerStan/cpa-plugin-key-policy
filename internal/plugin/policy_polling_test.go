package plugin

import (
	"cpa-key-policy/internal/policy"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitForPolledPolicy(t *testing.T, condition func() bool) {
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
			t.Fatal("policy polling did not refresh cache")
		case <-tick.C:
		}
	}
}
func TestJSONPollingRefreshesPolicyAndCatalog(t *testing.T) {
	app := configureSettingsApp(t, filepath.Join(t.TempDir(), "policy.json"))
	path := app.store.StatePath()
	state, err := policy.LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := policy.HashKey("test-polled-key")
	state.Keys = []policy.KeyConfig{{ID: "polled", Enabled: true, KeyHash: hash, IncludeModels: []string{"ustc/*"}}}
	state.CatalogGroups = json.RawMessage(`[{"name":"ustc","keys":["polled"],"include_models":["ustc/*"]}]`)
	raw, _ := json.Marshal(state)
	tmp := path + ".new"
	if err = os.WriteFile(tmp, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	waitForPolledPolicy(t, func() bool {
		return app.store.FindByAPIKey("test-polled-key") != nil && len(app.catalogPolicySnapshot().Groups) == 1
	})
	state.Keys = nil
	state.CatalogGroups = json.RawMessage(`[]`)
	raw, _ = json.Marshal(state)
	os.WriteFile(tmp, raw, 0600)
	os.Rename(tmp, path)
	waitForPolledPolicy(t, func() bool {
		return app.store.FindByAPIKey("test-polled-key") == nil && len(app.catalogPolicySnapshot().Groups) == 0
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
	waitForPolledPolicy(t, func() bool { return app.policyStatus()["policy_reload_error"] != "" })
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

func TestPollIntervalRegistration(t *testing.T) {
	cfg, err := decodeRegistration(nil)
	if err != nil || cfg.PollInterval != "5s" {
		t.Fatalf("default interval: %+v %v", cfg, err)
	}
	cfg, err = decodeRegistration([]byte("poll_interval: 250ms\n"))
	if err != nil || cfg.PollInterval != "250ms" {
		t.Fatalf("configured interval: %+v %v", cfg, err)
	}
	for _, value := range []string{"0s", "-1s", "invalid"} {
		if _, err := decodeRegistration([]byte("poll_interval: " + value + "\n")); err == nil {
			t.Fatalf("accepted invalid interval: %s", value)
		}
	}
}
func TestPollingWaitsForIntervalAndAllowsExplicitReload(t *testing.T) {
	app := configureSettingsApp(t, filepath.Join(t.TempDir(), "policy.json"))
	if err := app.startPolicyPolling("1h"); err != nil {
		t.Fatal(err)
	}
	path := app.store.StatePath()
	state, err := policy.LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := policy.HashKey("polling-only-key")
	state.Keys = []policy.KeyConfig{{ID: "polling-only", Enabled: true, KeyHash: hash}}
	raw, _ := json.Marshal(state)
	tmp := path + ".new"
	if err = os.WriteFile(tmp, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	if app.store.FindByAPIKey("polling-only-key") != nil {
		t.Fatal("file edit refreshed cache before the scheduled check")
	}
	if app.policyStatus()["poll_interval"] != "1h0m0s" {
		t.Fatal("configured polling interval not applied")
	}
	if err = app.reloadJSONPolicy(); err != nil {
		t.Fatal(err)
	}
	if app.store.FindByAPIKey("polling-only-key") == nil {
		t.Fatal("explicit reload did not refresh cache")
	}
}
