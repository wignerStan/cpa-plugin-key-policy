package policy

import (
	"errors"
	"strings"
	"time"
)

func (s *Store) SetGlobalWeightedRoundRobin(enabled bool) error {
	return s.mutatePolicy(func(cfg *Config) error { cfg.GlobalWeightedRoundRobin = enabled; return nil })
}

// UpsertKey commits policy before updating the query cache.
func (s *Store) UpsertKey(input KeyConfig) error {
	return s.mutatePolicy(func(cfg *Config) error {
		now := time.Now().UTC()
		input.UpdatedAt = now
		for i, k := range cfg.Keys {
			if k.ID == input.ID {
				input.CreatedAt = k.CreatedAt
				cfg.Keys[i] = input
				return nil
			}
		}
		if input.CreatedAt.IsZero() {
			input.CreatedAt = now
		}
		cfg.Keys = append(cfg.Keys, input)
		return nil
	})
}
func (s *Store) DeleteKey(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errPolicyIDRequired
	}
	err := s.mutatePolicy(func(cfg *Config) error {
		for i, k := range cfg.Keys {
			if k.ID == id {
				cfg.Keys = append(cfg.Keys[:i], cfg.Keys[i+1:]...)
				return nil
			}
		}
		return ErrUnknownKey
	})
	if err == nil {
		_ = s.ResetRPM(id)
	}
	return err
}
func (s *Store) RotateKey(id string) (string, KeyConfig, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", KeyConfig{}, errPolicyIDRequired
	}
	plain, err := GenerateKey()
	if err != nil {
		return "", KeyConfig{}, err
	}
	hash, err := HashKey(plain)
	if err != nil {
		return "", KeyConfig{}, err
	}
	err = s.mutatePolicy(func(cfg *Config) error {
		for i := range cfg.Keys {
			if cfg.Keys[i].ID == id {
				cfg.Keys[i].Key = ""
				cfg.Keys[i].KeyHash = hash
				cfg.Keys[i].UpdatedAt = time.Now().UTC()
				return nil
			}
		}
		return ErrUnknownKey
	})
	if err != nil {
		return "", KeyConfig{}, err
	}
	_ = s.ResetRPM(id)
	for _, k := range s.Keys() {
		if k.ID == id {
			return plain, k, nil
		}
	}
	return "", KeyConfig{}, ErrUnknownKey
}
func (s *Store) UpsertAlias(input AliasMapping) error {
	return s.mutatePolicy(func(cfg *Config) error {
		for i, k := range cfg.Aliases {
			if strings.EqualFold(k.Alias, input.Alias) {
				cfg.Aliases[i] = input
				return nil
			}
		}
		cfg.Aliases = append(cfg.Aliases, input)
		return nil
	})
}
func (s *Store) DeleteAlias(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("alias name is required")
	}
	return s.mutatePolicy(func(cfg *Config) error {
		for _, k := range cfg.Keys {
			for _, ref := range k.Aliases {
				if strings.EqualFold(ref.Alias, name) {
					return errors.New("alias is referenced by a key; remove references first")
				}
			}
		}
		for i, a := range cfg.Aliases {
			if strings.EqualFold(a.Alias, name) {
				cfg.Aliases = append(cfg.Aliases[:i], cfg.Aliases[i+1:]...)
				break
			}
		}
		return nil
	})
}
func (s *Store) UpsertClassifyRule(input ClassifyRule) error {
	return s.mutatePolicy(func(cfg *Config) error {
		for i, k := range cfg.ClassifyRules {
			if strings.EqualFold(k.Name, input.Name) {
				cfg.ClassifyRules[i] = input
				return nil
			}
		}
		cfg.ClassifyRules = append(cfg.ClassifyRules, input)
		return nil
	})
}
func (s *Store) DeleteClassifyRule(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("rule name is required")
	}
	return s.mutatePolicy(func(cfg *Config) error {
		for i, k := range cfg.ClassifyRules {
			if strings.EqualFold(k.Name, name) {
				cfg.ClassifyRules = append(cfg.ClassifyRules[:i], cfg.ClassifyRules[i+1:]...)
				break
			}
		}
		return nil
	})
}
func (s *Store) ReorderClassifyRules(names []string) error {
	return s.mutatePolicy(func(cfg *Config) error {
		var out []ClassifyRule
		used := map[string]bool{}
		for _, n := range names {
			n = strings.ToLower(n)
			if used[n] {
				continue
			}
			for _, r := range cfg.ClassifyRules {
				if strings.EqualFold(r.Name, n) {
					out = append(out, r)
					used[n] = true
					break
				}
			}
		}
		for _, r := range cfg.ClassifyRules {
			if !used[strings.ToLower(r.Name)] {
				out = append(out, r)
			}
		}
		cfg.ClassifyRules = out
		return nil
	})
}

// PatchKey applies a partial update to the latest authoritative key, not a stale cache copy.
func (s *Store) PatchKey(id string, change func(*KeyConfig) error) error {
	return s.mutatePolicy(func(cfg *Config) error {
		for i := range cfg.Keys {
			if cfg.Keys[i].ID == id {
				if err := change(&cfg.Keys[i]); err != nil {
					return err
				}
				cfg.Keys[i].UpdatedAt = time.Now().UTC()
				return nil
			}
		}
		return ErrUnknownKey
	})
}
