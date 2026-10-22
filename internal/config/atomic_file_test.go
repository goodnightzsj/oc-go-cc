package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationPreservesConfigSymlink(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target.json"), filepath.Join(dir, "config.json")
	if err := os.WriteFile(target, []byte(`{"api_key":"synthetic-key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateConfigFileOnDisk(link); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("startup migration replaced the config symlink with an independent regular file")
	}
	data, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(data), `"api_keys"`) {
		t.Fatal("symlink target was not migrated")
	}
	backups, err := filepath.Glob(target + ".pre-key-pool-migration-*")
	if err != nil || len(backups) != 1 {
		t.Fatal("target backup missing")
	}
	backup, err := os.ReadFile(backups[0])
	if err != nil || string(backup) != `{"api_key":"synthetic-key"}` {
		t.Fatal("backup did not preserve original")
	}
}

func TestAtomicConfigWriteFailurePreservesTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nonempty")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(target, "keep")
	if err := os.WriteFile(kept, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(target, []byte("replacement")); err == nil {
		t.Fatal("rename onto directory must fail")
	}
	data, err := os.ReadFile(kept)
	if err != nil || string(data) != "original" {
		t.Fatal("failed commit damaged original")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".routatic-config-") {
			t.Fatal("temporary file leaked")
		}
	}
}
