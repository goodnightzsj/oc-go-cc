package storage

import (
	"path/filepath"
	"testing"
)

// TestOpen_WALAndBusyTimeoutActuallyApply guards a silent misconfiguration:
// modernc.org/sqlite only parses the _pragma, _time_format and _txlock DSN
// parameters, so the mattn-style "?_journal_mode=WAL&_busy_timeout=5000" form
// this used to build was ignored outright — the database ran in delete-journal
// mode with a zero busy timeout, so any contended write failed immediately.
func TestOpen_WALAndBusyTimeoutActuallyApply(t *testing.T) {
	db, err := Open(Config{
		DatabasePath: filepath.Join(t.TempDir(), "wal.db"),
		WALEnabled:   true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	var journalMode string
	if err := db.DB().QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var busyTimeout int
	if err := db.DB().QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
	}
}

// The busy timeout must apply even when WAL is disabled, since contention is
// exactly the case the timeout exists for.
func TestOpen_BusyTimeoutWithoutWAL(t *testing.T) {
	db, err := Open(Config{
		DatabasePath: filepath.Join(t.TempDir(), "nowal.db"),
		WALEnabled:   false,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	var busyTimeout int
	if err := db.DB().QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
	}
}
