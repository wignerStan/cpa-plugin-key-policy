package plugin

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cpa-key-policy/internal/policy"
	"gopkg.in/yaml.v3"
)

// rewritePlainKeysInConfig replaces only plaintext key fields in the host
// configuration. The plugin receives ConfigYAML (the plugin subtree), but the
// host owns the original file, so ConfigPath is required for this migration.
// Missing paths are ignored for home/cloud deployments without local files.
func rewritePlainKeysInConfig(configPath string, cfg policy.Config) error {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return nil
	}

	hashes := make(map[string]string)
	for _, key := range cfg.Keys {
		id := strings.TrimSpace(key.ID)
		plain := strings.TrimSpace(key.Key)
		hash := strings.TrimSpace(key.KeyHash)
		if id == "" || plain == "" || strings.HasPrefix(plain, policy.HashPrefix) || hash == "" {
			continue
		}
		hashes[id] = hash
	}
	if len(hashes) == 0 {
		return nil
	}

	raw, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return err
	}
	if !rewritePlainKeyNodes(&document, hashes) {
		return nil
	}

	var encoded bytes.Buffer
	encoder := yaml.NewEncoder(&encoded)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		_ = encoder.Close()
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	if bytes.Equal(raw, encoded.Bytes()) {
		return nil
	}
	return atomicWriteConfig(configPath, encoded.Bytes())
}

func rewritePlainKeyNodes(document *yaml.Node, hashes map[string]string) bool {
	root := document
	if root != nil && root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	plugins, ok := mappingValue(root, "plugins")
	if !ok {
		return false
	}
	configs, ok := mappingValue(plugins, "configs")
	if !ok {
		return false
	}
	pluginConfig, ok := mappingValue(configs, PluginID)
	if !ok {
		return false
	}
	keys, ok := mappingValue(pluginConfig, "keys")
	if !ok || keys.Kind != yaml.SequenceNode {
		return false
	}

	changed := false
	for _, item := range keys.Content {
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		idNode, _, ok := mappingValueWithIndex(item, "id")
		if !ok {
			continue
		}
		hash, ok := hashes[strings.TrimSpace(idNode.Value)]
		if !ok {
			continue
		}
		_, keyIndex, hasKey := mappingValueWithIndex(item, "key")
		hashNode, _, hasHash := mappingValueWithIndex(item, "key_hash")
		if !hasKey {
			continue
		}
		if hasHash {
			hashNode.Tag = "!!str"
			hashNode.Value = hash
			item.Content = append(item.Content[:keyIndex], item.Content[keyIndex+2:]...)
		} else {
			item.Content[keyIndex].Tag = "!!str"
			item.Content[keyIndex].Value = "key_hash"
			item.Content[keyIndex+1].Tag = "!!str"
			item.Content[keyIndex+1].Value = hash
		}
		changed = true
	}
	return changed
}

func mappingValue(node *yaml.Node, key string) (*yaml.Node, bool) {
	value, _, ok := mappingValueWithIndex(node, key)
	return value, ok
}

func mappingValueWithIndex(node *yaml.Node, key string) (*yaml.Node, int, bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, -1, false
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index] != nil && node.Content[index].Value == key {
			return node.Content[index+1], index, true
		}
	}
	return nil, -1, false
}

func atomicWriteConfig(path string, raw []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("config path is a directory")
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".hash-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}
