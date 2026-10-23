package plugin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"cpa-key-policy/internal/policy"
	"github.com/wignerStan/infra/go/files/notify"
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

type policyNotifications struct {
	subscription *notify.FileSubscription
	cancel       context.CancelFunc
	done         chan struct{}
	once         sync.Once
}

func (a *App) startPolicyNotifications() error {
	path := a.store.StatePath()
	subscription, err := notify.SubscribeFile(path)
	if err != nil {
		return err
	}
	digest, err := policyDigest(path)
	if err != nil {
		subscription.Close()
		return err
	}
	a.stopPolicyNotifications()
	ctx, cancel := context.WithCancel(context.Background())
	n := &policyNotifications{subscription: subscription, cancel: cancel, done: make(chan struct{})}
	a.notificationMu.Lock()
	a.notifications = n
	a.notificationMu.Unlock()
	go func() {
		defer close(n.done)
		for {
			_, err := subscription.Read(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, notify.ErrClosed) {
					return
				}
				a.setReloadError(err)
				continue
			}
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
	}()
	// The OS subscription is established before reading the current document,
	// so replacement during registration cannot leave the cache stale.
	if err = a.reloadJSONPolicy(); err != nil {
		a.stopPolicyNotifications()
		return err
	}
	return nil
}
func (a *App) stopPolicyNotifications() {
	a.notificationMu.Lock()
	n := a.notifications
	a.notifications = nil
	a.notificationMu.Unlock()
	if n != nil {
		n.once.Do(func() { n.cancel(); _ = n.subscription.Close() })
		<-n.done
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
