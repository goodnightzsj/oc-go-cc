package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The whole point of migrating from the file's own bytes is that a ${VAR}
// placeholder survives. Rewriting from a loaded Config would put the resolved
// secret on disk, which is the one thing the config file must never contain.
func TestMigrateKeyPoolsKeepsPlaceholders(t *testing.T) {
	original := `{
  "api_key": "${ROUTATIC_PROXY_API_KEY}",
  "api_keys": ["literal-key"],
  "cline_pass": {
    "api_key": "${CLINE_KEY}",
    "api_keys": []
  }
}`
	migrated, changed, err := MigrateKeyPools([]byte(original))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("a config with a single key must be reported as migrated")
	}
	text := string(migrated)
	if !strings.Contains(text, "${ROUTATIC_PROXY_API_KEY}") || !strings.Contains(text, "${CLINE_KEY}") {
		t.Fatalf("migration lost a placeholder:\n%s", text)
	}
	if strings.Contains(text, `"api_key": "${`) {
		t.Fatalf("a placeholder was left in the single-key field:\n%s", text)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(migrated, &root); err != nil {
		t.Fatal(err)
	}
	var globalKeys []string
	if err := json.Unmarshal(root["api_keys"], &globalKeys); err != nil {
		t.Fatal(err)
	}
	// The single key leads, matching appendAPIKeys, so migrating cannot change
	// which key serves the first request.
	want := []string{"${ROUTATIC_PROXY_API_KEY}", "literal-key"}
	if len(globalKeys) != len(want) || globalKeys[0] != want[0] || globalKeys[1] != want[1] {
		t.Fatalf("global pool = %v, want %v", globalKeys, want)
	}
	var globalSingle string
	if err := json.Unmarshal(root["api_key"], &globalSingle); err != nil {
		t.Fatal(err)
	}
	if globalSingle != "" {
		t.Fatalf("api_key = %q, want it emptied", globalSingle)
	}

	var cline map[string]json.RawMessage
	if err := json.Unmarshal(root["cline_pass"], &cline); err != nil {
		t.Fatal(err)
	}
	var clineKeys []string
	if err := json.Unmarshal(cline["api_keys"], &clineKeys); err != nil {
		t.Fatal(err)
	}
	if len(clineKeys) != 1 || clineKeys[0] != "${CLINE_KEY}" {
		t.Fatalf("cline_pass pool = %v, want [${CLINE_KEY}]", clineKeys)
	}
}

// A config that is already in the new shape must be left byte-identical, so a
// restart cannot rewrite (and reorder) a file the operator did not change.
func TestMigrateKeyPoolsLeavesAMigratedFileAlone(t *testing.T) {
	original := `{
  "api_key": "",
  "api_keys": ["only-key"],
  "cline_pass": {"api_key": "", "api_keys": ["a", "b"]}
}`
	migrated, changed, err := MigrateKeyPools([]byte(original))
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatalf("an already-migrated file was rewritten:\n%s", migrated)
	}
	if string(migrated) != original {
		t.Fatal("an unchanged migration must return the input untouched")
	}
}

// A config with no single key anywhere is not migrated and not rewritten.
func TestMigrateKeyPoolsIgnoresAConfigWithNoSingleKey(t *testing.T) {
	original := `{"api_keys": ["k1", "k2"], "host": "127.0.0.1"}`
	if _, changed, err := MigrateKeyPools([]byte(original)); err != nil || changed {
		t.Fatalf("changed=%v err=%v, want no change", changed, err)
	}
}

// The on-disk migration must leave a readable backup of the original before it
// writes anything, because the migration empties a field the operator cannot
// reconstruct once the process has started.
func TestMigrateConfigFileOnDiskBacksUpBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	original := `{"api_key": "${K}", "api_keys": [], "host": "127.0.0.1"}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	migrated, err := MigrateConfigFileOnDisk(path)
	if err != nil {
		t.Fatal(err)
	}
	if !migrated {
		t.Fatal("expected the file to be migrated")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var backups []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "config.json.pre-key-pool-migration-") {
			backups = append(backups, entry.Name())
		}
	}
	if len(backups) != 1 {
		t.Fatalf("backups = %v, want exactly one", backups)
	}
	backupData, err := os.ReadFile(filepath.Join(dir, backups[0]))
	if err != nil {
		t.Fatal(err)
	}
	if string(backupData) != original {
		t.Fatalf("backup is not the original file:\n%s", backupData)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), `"api_key": "${K}"`) {
		t.Fatalf("the file was not migrated:\n%s", after)
	}

	// The migrated file must still load, and still resolve to the same pool.
	t.Setenv("K", "resolved-key")
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("the migrated file does not load: %v", err)
	}
	if keys := cfg.EffectiveAPIKeys(); len(keys) != 1 || keys[0] != "resolved-key" {
		t.Fatalf("EffectiveAPIKeys() = %v, want [resolved-key]", keys)
	}

	// Running it again must be a no-op, so a restart does not churn backups.
	again, err := MigrateConfigFileOnDisk(path)
	if err != nil {
		t.Fatal(err)
	}
	if again {
		t.Fatal("a second migration rewrote an already-migrated file")
	}
}

// A config the loader would reject must not be rewritten by the migration: the
// loader's own error is the message an operator needs to see.
func TestMigrateKeyPoolsLeavesInvalidJSONToTheLoader(t *testing.T) {
	if _, _, err := MigrateKeyPools([]byte("not-json")); err == nil {
		t.Fatal("expected an error for unparseable config")
	}
}

// A platform block that is not an object cannot hold a key pair, and must not
// stop the global pool from being migrated.
func TestMigrateKeyPoolsSkipsANonObjectBlock(t *testing.T) {
	original := `{"api_key": "global-key", "cline_pass": "not-an-object"}`
	migrated, changed, err := MigrateKeyPools([]byte(original))
	if err != nil {
		t.Fatalf("a malformed block must not fail the migration: %v", err)
	}
	if !changed {
		t.Fatal("the global pool must still be migrated")
	}
	if !strings.Contains(string(migrated), `"global-key"`) {
		t.Fatalf("the global key was lost:\n%s", migrated)
	}
	if !strings.Contains(string(migrated), `"not-an-object"`) {
		t.Fatalf("the malformed block was altered:\n%s", migrated)
	}
}
