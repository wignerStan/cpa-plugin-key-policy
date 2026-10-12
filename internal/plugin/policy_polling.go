package plugin

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"cpa-key-policy/internal/policy"
	"gopkg.in/yaml.v3"
)

type pluginRegistration struct {
	Enabled      *bool  `yaml:"enabled"`
	Priority     int    `yaml:"priority"`
	StateFile    string `yaml:"state_file"`
	PollInterval string `yaml:"poll_interval"`
}

func decodeRegistration(raw []byte) (pluginRegistration, error) {
	cfg := pluginRegistration{StateFile: policy.DefaultConfig().StateFile, PollInterval: "5s"}
	if len(bytes.TrimSpace(raw)) == 0 {
		return cfg, nil
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("plugin YAML accepts registration fields only; migrate settings into JSON: %w", err)
	}
	if cfg.StateFile == "" {
		cfg.StateFile = policy.DefaultConfig().StateFile
	}
	if cfg.PollInterval == "" {
		cfg.PollInterval = "5s"
	}
	if interval, err := time.ParseDuration(cfg.PollInterval); err != nil || interval <= 0 {
		return cfg, fmt.Errorf("poll_interval must be a positive duration, such as 5s")
	}
	return cfg, nil
}
func catalogFromState(state *policy.State) (catalogPolicy, error) {
	raw, err := json.Marshal(map[string]json.RawMessage{"catalog_groups": state.CatalogGroups})
	if err != nil {
		return catalogPolicy{}, err
	}
	return decodeCatalogPolicy(raw)
}
func (a *App) loadCatalogFromJSON() (catalogPolicy, error) {
	state, err := policy.LoadState(a.store.StatePath())
	if err != nil {
		return catalogPolicy{}, err
	}
	return catalogFromState(state)
}
func (a *App) reloadJSONPolicy() error {
	a.reloadMu.Lock()
	defer a.reloadMu.Unlock()
	state, err := policy.LoadState(a.store.StatePath())
	if err != nil {
		return err
	}
	catalog, err := catalogFromState(state)
	if err != nil {
		return err
	}
	if err = a.store.RefreshPolicy(state); err != nil {
		return err
	}
	a.setCatalogPolicy(catalog)
	return nil
}
func policyDigest(path string) ([32]byte, error) {
	state, err := policy.LoadState(path)
	if err != nil {
		return [32]byte{}, err
	}
	state.Usage = nil
	state.UpdatedAt = time.Time{}
	raw, err := json.Marshal(state)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

type policyPoller struct {
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	interval time.Duration
}

func (a *App) startPolicyPolling(intervalText string) error {
	interval, err := time.ParseDuration(intervalText)
	if err != nil || interval <= 0 {
		return fmt.Errorf("poll_interval must be a positive duration")
	}
	path := a.store.StatePath()
	digest, err := policyDigest(path)
	if err != nil {
		return err
	}
	a.stopPolicyPolling()
	p := &policyPoller{stop: make(chan struct{}), done: make(chan struct{}), interval: interval}
	a.pollMu.Lock()
	a.poller = p
	a.pollMu.Unlock()
	go func() {
		defer close(p.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-p.stop:
				return
			case <-ticker.C:
				next, err := policyDigest(path)
				if err != nil {
					a.setReloadError(err)
					continue
				}
				if next == digest {
					a.setReloadError(nil)
					continue
				}
				if err = a.reloadJSONPolicy(); err != nil {
					a.setReloadError(err)
					continue
				}
				digest = next
				a.setReloadError(nil)
			}
		}
	}()
	// Startup reads the current document immediately; later checks use the fixed interval.
	if err = a.reloadJSONPolicy(); err != nil {
		a.stopPolicyPolling()
		return err
	}
	return nil
}
func (a *App) stopPolicyPolling() {
	a.pollMu.Lock()
	p := a.poller
	a.poller = nil
	a.pollMu.Unlock()
	if p != nil {
		p.once.Do(func() { close(p.stop) })
		<-p.done
	}
}
func (a *App) setReloadError(err error) {
	a.pollMu.Lock()
	defer a.pollMu.Unlock()
	a.policyReloadError = ""
	if err != nil {
		a.policyReloadError = err.Error()
	}
}
func (a *App) policyStatus() map[string]any {
	out := a.store.Status()
	a.pollMu.Lock()
	out["policy_reload_error"] = a.policyReloadError
	if a.poller != nil {
		out["poll_interval"] = a.poller.interval.String()
	}
	a.pollMu.Unlock()
	return out
}
