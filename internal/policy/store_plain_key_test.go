package policy

import (
	"path/filepath"
	"testing"
)

func TestConfigurePlainKeyOverridesPersistedHash(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	oldHash, err := HashKey("cpa_old")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveState(statePath, []KeyConfig{{ID: "catalog", Name: "Catalog", Enabled: true, KeyHash: oldHash}}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	if err := store.Configure(Config{
		Enabled:   true,
		StateFile: statePath,
		Keys:      []KeyConfig{{ID: "catalog", Key: "cpa_new"}},
	}); err != nil {
		t.Fatal(err)
	}
	key := store.findByID("catalog")
	if key == nil || !MatchHash("cpa_new", key.KeyHash) || MatchHash("cpa_old", key.KeyHash) {
		t.Fatalf("plain key did not override state hash: %+v", key)
	}
	loaded, err := LoadState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Keys) != 1 || !MatchHash("cpa_new", loaded.Keys[0].KeyHash) {
		t.Fatalf("updated hash was not persisted: %+v", loaded.Keys)
	}
	if loaded.Keys[0].Key != "" {
		t.Fatal("plaintext key leaked into state")
	}
}
