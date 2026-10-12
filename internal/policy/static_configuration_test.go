package policy

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestYAMLPolicyOverridesLegacyJSONAndPreservesUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	oldHash, _ := HashKey("old-key")
	raw, _ := json.Marshal(State{Version: 1, Keys: []KeyConfig{{ID: "old", Enabled: true, KeyHash: oldHash}}, Aliases: []AliasMapping{{Alias: "old"}}, Usage: map[string]*UsageState{"live": {Daily: UsageWindow{TotalUSD: 3}}}})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	h, _ := HashKey("new-key")
	cfg := Config{Enabled: true, StateFile: path, Keys: []KeyConfig{{ID: "live", Enabled: true, KeyHash: h, IncludeModels: []string{"gpt-*"}, AllowModelsEndpoint: true}}}
	s := NewStore()
	if err := s.Configure(cfg); err != nil {
		t.Fatal(err)
	}
	if len(s.Keys()) != 1 || s.Keys()[0].ID != "live" || len(s.AliasesSnapshot()) != 0 {
		t.Fatal("JSON policy overrode YAML")
	}
	if s.usageSnapshotLocked()["live"].Daily.TotalUSD != 3 {
		t.Fatal("usage lost")
	}
	d := s.Authenticate("POST", "/v1/responses", http.Header{"Authorization": []string{"Bearer new-key"}}, nil, []byte(`{"model":"gpt-new"}`))
	if !d.Allowed {
		t.Fatalf("YAML key rejected: %+v", d)
	}
	if err := s.FlushUsage(); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	var doc map[string]any
	json.Unmarshal(raw, &doc)
	for _, k := range []string{"keys", "aliases", "classify_rules", "global_weighted_round_robin"} {
		if _, ok := doc[k]; ok {
			t.Fatalf("static field persisted: %s", k)
		}
	}
	cfg.Keys = nil
	if err := s.Configure(cfg); err != nil {
		t.Fatal(err)
	}
	if len(s.Keys()) != 0 {
		t.Fatal("removed YAML key resurrected")
	}
	if s.usageSnapshotLocked()["live"].Daily.TotalUSD != 3 {
		t.Fatal("revocation lost usage")
	}
}
