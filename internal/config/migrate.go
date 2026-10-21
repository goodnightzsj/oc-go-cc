package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MigrateKeyPools collapses every platform's two credential fields into the one
// array field, and returns the config file's new bytes. It reports whether
// anything changed.
//
// It works on the file's own JSON rather than on a loaded Config, because a
// loaded Config no longer knows which values were written as ${VAR} placeholders
// and which were literals: interpolation runs before unmarshalling. Rewriting
// from the parsed form would replace every placeholder with the secret it
// resolved to, which is the one thing the config file must never contain. So
// the single key is moved across as the text the file already had.
//
// The pool order is the single key first, then the array, matching what
// appendAPIKeys produces at runtime - so a migration cannot reorder which key
// serves the first request.
func MigrateKeyPools(data []byte) ([]byte, bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, false, fmt.Errorf("parsing config: %w", err)
	}

	changed := false
	// The global pool is a top-level pair; each platform's sits in its block.
	if moved, err := migrateKeyPoolFields(root); err != nil {
		return nil, false, err
	} else if moved {
		changed = true
	}

	for _, row := range keyPoolFields {
		if row.jsonKey == "" {
			continue
		}
		raw, ok := root[row.jsonKey]
		if !ok {
			continue
		}
		var block map[string]json.RawMessage
		if err := json.Unmarshal(raw, &block); err != nil {
			// A block that is not an object cannot hold a key pair; leave it
			// for the loader to reject with its own message.
			continue
		}
		moved, err := migrateKeyPoolFields(block)
		if err != nil {
			return nil, false, fmt.Errorf("%s: %w", row.jsonKey, err)
		}
		if !moved {
			continue
		}
		encoded, err := json.Marshal(block)
		if err != nil {
			return nil, false, fmt.Errorf("encoding %s: %w", row.jsonKey, err)
		}
		root[row.jsonKey] = encoded
		changed = true
	}

	if !changed {
		return data, false, nil
	}
	// MarshalIndent sorts map keys, so a migrated file comes out with its
	// fields in the same order a settings save writes them.
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("encoding config: %w", err)
	}
	return append(out, '\n'), true, nil
}

// migrateKeyPoolFields moves "api_key" into "api_keys" within one object,
// leaving "api_key" present but empty. It reports whether it changed anything.
//
// The field is emptied rather than deleted so a reader that still looks for it
// finds nothing rather than a missing key, and so a config with no array yet
// keeps both names visible to an operator editing the file by hand.
func migrateKeyPoolFields(obj map[string]json.RawMessage) (bool, error) {
	singleRaw, hasSingle := obj["api_key"]
	if !hasSingle {
		return false, nil
	}
	var single string
	if err := json.Unmarshal(singleRaw, &single); err != nil {
		// Not a string: not a credential this migration understands.
		return false, nil
	}
	if single == "" {
		// Already migrated, or never set. Nothing to move.
		return false, nil
	}

	var many []string
	if arrayRaw, ok := obj["api_keys"]; ok {
		if err := json.Unmarshal(arrayRaw, &many); err != nil {
			return false, fmt.Errorf("api_keys is not an array of strings: %w", err)
		}
	}

	// Reuse the runtime merge so the migrated order cannot drift from the order
	// the proxy actually uses.
	pool := appendAPIKeys(single, many)
	encoded, err := json.Marshal(pool)
	if err != nil {
		return false, fmt.Errorf("encoding api_keys: %w", err)
	}
	obj["api_keys"] = encoded
	obj["api_key"] = json.RawMessage(`""`)
	return true, nil
}

// FoldKeyPoolBlocksInPlace rewrites a raw config object so every key-pool block
// has its single-key field emptied and its array holding the merged pool. It is
// the same fold MigrateKeyPools performs, exposed for a caller that is about to
// resolve row indices against the config.
//
// The settings save path needs it: the dashboard renders the merged pool, so a
// row index counts from the single key's position. Resolving that index against
// a stored file that still separates the two fields would address the wrong
// credential - or refuse a correct edit as stale.
func FoldKeyPoolBlocksInPlace(root map[string]json.RawMessage) error {
	if _, err := migrateKeyPoolFields(root); err != nil {
		return err
	}
	for _, row := range keyPoolFields {
		if row.jsonKey == "" {
			continue
		}
		raw, ok := root[row.jsonKey]
		if !ok {
			continue
		}
		var block map[string]json.RawMessage
		if err := json.Unmarshal(raw, &block); err != nil {
			continue
		}
		moved, err := migrateKeyPoolFields(block)
		if err != nil {
			return fmt.Errorf("%s: %w", row.jsonKey, err)
		}
		if !moved {
			continue
		}
		encoded, err := json.Marshal(block)
		if err != nil {
			return fmt.Errorf("encoding %s: %w", row.jsonKey, err)
		}
		root[row.jsonKey] = encoded
	}
	return nil
}

// BackupConfigFile writes a timestamped copy of data next to path and returns
// the backup's path. Callers take it before a write that will fold a key pool,
// for the same reason the startup migration does: the fold empties a field, and
// once the process has started there is no other way to reconstruct the file
// that held it.
func BackupConfigFile(path string, data []byte) (string, error) {
	backup := path + ".pre-key-pool-migration-" + time.Now().UTC().Format("20060102150405")
	if err := os.WriteFile(backup, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write config backup %s: %w", backup, err)
	}
	return backup, nil
}

// MigrateConfigFileOnDisk collapses the key pools in the config file at path,
// backing the original up first. It reports whether it wrote anything.
//
// The backup is taken before any write and kept even on success: the migration
// empties a field, and an operator who needs the pre-migration file back has no
// other way to reconstruct it once the process has started. A failed write
// leaves the original file untouched, because the write is the atomic
// rename-and-replace the settings path already uses.
func MigrateConfigFileOnDisk(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	migrated, changed, err := MigrateKeyPools(data)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}

	if _, err := BackupConfigFile(path, data); err != nil {
		return false, err
	}
	if err := writeFileAtomic(path, migrated); err != nil {
		return false, fmt.Errorf("failed to write migrated config: %w", err)
	}
	return true, nil
}

// writeFileAtomic replaces path through a temporary file in the same directory,
// so a crash mid-write cannot leave a truncated config behind.
func writeFileAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".routatic-config-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	defer func() { _ = file.Close() }()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
