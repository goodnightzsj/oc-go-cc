package storage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenRestrictsDatabaseFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not enforced on Windows")
	}

	path := filepath.Join(t.TempDir(), "private.db")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatalf("precreate database: %v", err)
	}

	db, err := Open(Config{DatabasePath: path, WALEnabled: true})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Stat(candidate)
		if err != nil {
			t.Fatalf("stat %s: %v", filepath.Base(candidate), err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Errorf("%s mode = %04o, want 0600", filepath.Base(candidate), got)
		}
	}
}

func TestOpenClearsLegacyCatalogAPIKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := Open(Config{DatabasePath: path})
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := db.DB().ExecContext(context.Background(), `
		INSERT INTO providers (name, base_url, api_key) VALUES ('legacy', 'https://example.com', 'plaintext-secret')
	`); err != nil {
		t.Fatalf("seed legacy key: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	db, err = Open(Config{DatabasePath: path})
	if err != nil {
		t.Fatalf("reopen legacy database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var key *string
	if err := db.DB().QueryRow(`SELECT api_key FROM providers WHERE name = 'legacy'`).Scan(&key); err != nil {
		t.Fatalf("read legacy key: %v", err)
	}
	if key != nil {
		t.Errorf("legacy catalog API key = %q, want NULL", *key)
	}
}
