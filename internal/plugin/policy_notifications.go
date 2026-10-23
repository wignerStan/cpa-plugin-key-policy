package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cpa-key-policy/internal/policy"
	configfile "github.com/wignerStan/infra/go/config/file"
	"gopkg.in/yaml.v3"
)

type pluginRegistration struct {
	Enabled   *bool  `yaml:"enabled"`
	Priority  int    `yaml:"priority"`
	StateFile string `yaml:"state_file"`
}

func decodeRegistration(raw []byte) (pluginRegistration, error) {
	cfg := pluginRegistration{StateFile: policy.DefaultConfig().StateFile}
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
	var catalog catalogPolicy
	err := a.store.ReloadPolicy(func(state *policy.State) error {
		var err error
		catalog, err = catalogFromState(state)
		return err
	})
	if err != nil {
		return err
	}
	a.setCatalogPolicy(catalog)
	return nil
}
func policySnapshot(path string) (configfile.Snapshot, error) {
	state, err := policy.LoadState(path)
	if err != nil {
		return configfile.Snapshot{}, err
	}
	state.Usage = nil
	state.UpdatedAt = time.Time{}
	return configfile.NewSnapshot(state, state.NeedsRewrite())
}

func (a *App) startPolicyNotifications() error {
	a.stopPolicyNotifications()
	path := a.store.StatePath()
	subscription, err := configfile.Observe(context.Background(), path, configfile.Observer{
		Inspect: func() (configfile.Snapshot, error) { return policySnapshot(path) },
		Apply:   a.reloadJSONPolicy,
		Report:  a.setReloadError,
	})
	if err != nil {
		return err
	}
	a.notificationMu.Lock()
	a.notifications = subscription
	a.notificationMu.Unlock()
	return nil
}

func (a *App) stopPolicyNotifications() {
	a.notificationMu.Lock()
	subscription := a.notifications
	a.notifications = nil
	a.notificationMu.Unlock()
	if subscription != nil {
		_ = subscription.Close()
	}
}

func (a *App) setReloadError(err error) {
	a.notificationMu.Lock()
	defer a.notificationMu.Unlock()
	a.policyReloadError = ""
	if err != nil {
		a.policyReloadError = err.Error()
	}
}
func (a *App) policyStatus() map[string]any {
	out := a.store.Status()
	a.notificationMu.Lock()
	out["policy_reload_error"] = a.policyReloadError
	a.notificationMu.Unlock()
	return out
}
