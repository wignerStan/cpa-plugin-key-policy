package plugin

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"cpa-key-policy/internal/policy"
	"github.com/fsnotify/fsnotify"
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

type policyNotifications struct {
	watcher *fsnotify.Watcher
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func (a *App) startPolicyNotifications() error {
	path := a.store.StatePath()
	digest, err := policyDigest(path)
	if err != nil {
		return err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err = watcher.Add(filepath.Dir(path)); err != nil {
		watcher.Close()
		return err
	}
	n := &policyNotifications{watcher: watcher, stop: make(chan struct{}), done: make(chan struct{})}
	a.stopPolicyNotifications()
	a.notificationMu.Lock()
	a.notifications = n
	a.notificationMu.Unlock()
	go func() {
		defer close(n.done)
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		defer timer.Stop()
		for {
			select {
			case <-n.stop:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Clean(event.Name) != path {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
					continue
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(30 * time.Millisecond)
			case <-timer.C:
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
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				a.setReloadError(err)
			}
		}
	}()
	// Close the registration race: a write between initial load and watcher creation
	// must be reflected before requests proceed.
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
		n.once.Do(func() { close(n.stop); _ = n.watcher.Close() })
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
