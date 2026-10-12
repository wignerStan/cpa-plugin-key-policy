package policy

import (
	"path/filepath"
	"testing"
)

// TestUpsertKeyMultiTargetAliasPopulatesAliases reproduces the bug where
// creating a key with a multi-target alias (same alias name, different
// targets) results in aliases=[] even though models has 2 entries.
func TestUpsertKeyMultiTargetAliasPopulatesAliases(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	s := NewStore()
	if err := s.Configure(Config{Enabled: true, StateFile: stateFile}); err != nil {
		t.Fatal(err)
	}

	// Create a key with a multi-target alias "test" (2 entries, same alias).
	input := KeyConfig{
		ID:            "mt-key",
		Enabled:       true,
		RPM:           0,
		DailyLimitUSD: 0,
		Models: []ModelRule{
			{Alias: "test", Provider: "nvidia", TargetModel: "z-ai/glm-5.2", BillingMode: "tokens"},
			{Alias: "test", Provider: "opencode", TargetModel: "glm-5.2", BillingMode: "tokens"},
		},
	}
	if err := s.UpsertKey(input); err != nil {
		t.Fatalf("UpsertKey: %v", err)
	}

	key, ok := s.findKeyLocked("mt-key")
	if !ok {
		t.Fatal("key not found")
	}
	t.Logf("after upsert: aliases=%+v models=%+v", key.Aliases, key.Models)
	if len(key.Aliases) != 1 {
		t.Fatalf("expected 1 alias ref (deduped), got %d: %+v", len(key.Aliases), key.Aliases)
	}
	if key.Aliases[0].Alias != "test" {
		t.Fatalf("expected alias ref 'test', got %q", key.Aliases[0].Alias)
	}
	if len(key.Models) != 2 {
		t.Fatalf("expected 2 resolved models, got %d: %+v", len(key.Models), key.Models)
	}
}

func (s *Store) findKeyLocked(id string) (*KeyConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.keys[id]
	return k, ok
}
