package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"cpa-key-policy/internal/policy"
	"gopkg.in/yaml.v3"
)

func TestRewritePlainKeysInConfig(t *testing.T) {
	plain := "cpa_plaintext_test"
	hash, err := policy.HashKey(plain)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := []byte("plugins:\n  configs:\n    cpa-key-policy:\n      keys:\n        - id: first\n          key: " + plain + "\n        - id: untouched\n          key: keep-me\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := policy.Config{Keys: []policy.KeyConfig{{ID: "first", Key: plain, KeyHash: hash}}}
	if err := rewritePlainKeysInConfig(path, cfg); err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Plugins struct {
			Configs map[string]struct {
				Keys []map[string]string `yaml:"keys"`
			} `yaml:"configs"`
		} `yaml:"plugins"`
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(got, &parsed); err != nil {
		t.Fatal(err)
	}
	keys := parsed.Plugins.Configs[PluginID].Keys
	if len(keys) != 2 {
		t.Fatalf("keys = %d, want 2", len(keys))
	}
	if keys[0]["key_hash"] != hash || keys[0]["key"] != "" {
		t.Fatalf("rewritten key = %#v, want only key_hash", keys[0])
	}
	if keys[1]["key"] != "keep-me" {
		t.Fatalf("unrelated key changed: %#v", keys[1])
	}
}
