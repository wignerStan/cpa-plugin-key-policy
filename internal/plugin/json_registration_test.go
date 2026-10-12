package plugin

import (
	"cpa-key-policy/internal/policy"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// Existing behavior fixtures describe policy in YAML for readability; only JSON
// is passed to the production policy loader, with registration-only host YAML.
func jsonRegistrationForTest(t *testing.T, raw []byte) []byte {
	t.Helper()
	cfg, err := policy.DecodeConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StateFile == policy.DefaultConfig().StateFile {
		cfg.StateFile = filepath.Join(t.TempDir(), "policy.json")
	}
	if _, err := os.Stat(cfg.StateFile); err == nil {
		out, err := yaml.Marshal(pluginRegistration{StateFile: cfg.StateFile})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if err = policy.SaveState(cfg.StateFile, cfg.Keys, nil, cfg.Aliases, cfg.ClassifyRules); err != nil {
		t.Fatal(err)
	}
	state, err := policy.LoadState(cfg.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	state.Enabled = &cfg.Enabled
	state.GlobalWeightedRoundRobin = &cfg.GlobalWeightedRoundRobin
	var source map[string]any
	if err = yaml.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if groups, ok := source["catalog_groups"]; ok {
		state.CatalogGroups, err = json.Marshal(groups)
		if err != nil {
			t.Fatal(err)
		}
	}
	doc, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(cfg.StateFile, doc, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(pluginRegistration{StateFile: cfg.StateFile})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
