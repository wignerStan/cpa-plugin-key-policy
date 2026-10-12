package policy

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Config returns policy from the authoritative document, never from host YAML.
func (state *State) Config(path string) Config {
	cfg := Config{Enabled: true, StateFile: path, Keys: state.Keys, Aliases: state.Aliases, ClassifyRules: state.ClassifyRules}
	if state.Enabled != nil {
		cfg.Enabled = *state.Enabled
	}
	if state.GlobalWeightedRoundRobin != nil {
		cfg.GlobalWeightedRoundRobin = *state.GlobalWeightedRoundRobin
	}
	return cfg
}
func canonicalKeys(keys []KeyConfig) []KeyConfig {
	out := append([]KeyConfig(nil), keys...)
	for i := range out {
		out[i].Models = nil
		out[i].Key = ""
	}
	return out
}

// RefreshPolicy publishes a validated JSON snapshot without replacing usage in flight.
func (s *Store) RefreshPolicy(state *State) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	cfg := state.Config(s.StatePath())
	if err := normalizeConfig(&cfg); err != nil {
		return err
	}
	if state.needsRewrite {
		s.persistMu.Lock()
		err := rewriteNormalizedState(s.StatePath(), state, cfg)
		s.persistMu.Unlock()
		if err != nil {
			return err
		}
	}
	s.publishPolicy(cfg)
	return nil
}
func (s *Store) publishPolicy(cfg Config) {
	s.mu.Lock()
	s.enabled = cfg.Enabled
	s.globalWeightedRoundRobin = cfg.GlobalWeightedRoundRobin
	s.aliases = make(map[string]*AliasMapping, len(cfg.Aliases))
	for i := range cfg.Aliases {
		item := cfg.Aliases[i]
		s.aliases[strings.ToLower(item.Alias)] = &item
	}
	s.keys = make(map[string]*KeyConfig, len(cfg.Keys))
	for i := range cfg.Keys {
		item := cfg.Keys[i]
		if len(item.Aliases) > 0 {
			item.Models = resolveAliasRefsToModels(item.Aliases, s.aliases)
		}
		s.keys[item.ID] = &item
	}
	s.classifyRules = cfg.ClassifyRules
	s.rebuildKeysByHashLocked()
	s.rrCounters = make(map[string]int)
	s.pendingPicks = make(map[string][]pendingPick)
	changed := s.onClassifyRulesChanged
	s.mu.Unlock()
	callClassifyRulesChanged(changed)
}

// mutatePolicy commits to JSON before notifying the in-memory query cache.
// updateMu serializes local policy writers; persistMu serializes the usage writer.
func (s *Store) mutatePolicy(change func(*Config) error) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.persistMu.Lock()
	defer s.persistMu.Unlock()
	path := s.StatePath()
	state, err := LoadState(path)
	if err != nil {
		return err
	}
	cfg := state.Config(path)
	if err = change(&cfg); err != nil {
		return err
	}
	if err = normalizeConfig(&cfg); err != nil {
		return err
	}
	state.Keys = canonicalKeys(cfg.Keys)
	state.Aliases = cfg.Aliases
	state.ClassifyRules = cfg.ClassifyRules
	state.Enabled = &cfg.Enabled
	state.GlobalWeightedRoundRobin = &cfg.GlobalWeightedRoundRobin
	state.Version = 3
	state.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err = atomicWriteStateFile(path, raw); err != nil {
		return err
	}
	s.publishPolicy(cfg)
	return nil
}

var errPolicyIDRequired = errors.New("id is required")

// UnmarshalJSON accepts a one-time plaintext key while MarshalJSON never emits it.
func (key *KeyConfig) UnmarshalJSON(raw []byte) error {
	type fields KeyConfig
	aux := struct {
		*fields
		Key string `json:"key"`
	}{fields: (*fields)(key)}
	if err := json.Unmarshal(raw, &aux); err != nil {
		return err
	}
	key.Key = aux.Key
	return nil
}
func rewriteNormalizedState(path string, state *State, cfg Config) error {
	state.Keys = canonicalKeys(cfg.Keys)
	state.Aliases = cfg.Aliases
	state.ClassifyRules = cfg.ClassifyRules
	state.Version = 3
	state.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteStateFile(path, raw)
}
