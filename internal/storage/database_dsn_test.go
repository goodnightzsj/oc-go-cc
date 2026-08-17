package storage

import (
	"database/sql"
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

// TestOpen_MigratesLegacySchemaIdempotently guards the column migration, which
// tolerates SQLite's "duplicate column name" error so it can run on every
// startup. A database written by an older build must gain every new column,
// keep its rows, and survive a second Open unchanged.
func TestOpen_MigratesLegacySchemaIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacy.Exec(`
		CREATE TABLE requests (
			id TEXT PRIMARY KEY, model TEXT NOT NULL, provider TEXT, scenario TEXT,
			start_time TIMESTAMP NOT NULL, duration_ms INTEGER,
			input_tokens INTEGER, output_tokens INTEGER, cost_usd REAL,
			streaming INTEGER, success INTEGER, error_msg TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO requests (id, model, start_time, cost_usd)
		VALUES ('legacy-1', 'glm-5.2', '2026-08-01T00:00:00Z', 0.5);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	for pass := 1; pass <= 2; pass++ {
		db, err := Open(Config{DatabasePath: path})
		if err != nil {
			t.Fatalf("open pass %d: %v", pass, err)
		}

		columns := map[string]bool{}
		rows, err := db.DB().Query(`SELECT name FROM pragma_table_info('requests')`)
		if err != nil {
			t.Fatalf("pass %d: read columns: %v", pass, err)
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatalf("pass %d: scan column: %v", pass, err)
			}
			columns[name] = true
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("pass %d: read columns: %v", pass, err)
		}
		_ = rows.Close()
		for _, want := range []string{
			"attempt", "cache_read_tokens", "cache_creation_tokens",
			"cost_usd", "cost_source", "details_known", "usage_trusted",
		} {
			if !columns[want] {
				t.Errorf("pass %d: column %q missing after migration", pass, want)
			}
		}

		var cost float64
		var source string
		if err := db.DB().QueryRow(`SELECT cost_usd, cost_source FROM requests WHERE id = 'legacy-1'`).Scan(&cost, &source); err != nil {
			t.Fatalf("pass %d: read migrated row: %v", pass, err)
		}
		if cost != 0.5 || source != CostSourceEstimated {
			t.Errorf("pass %d: legacy row = (%v, %q), want (0.5, %q)", pass, cost, source, CostSourceEstimated)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("close pass %d: %v", pass, err)
		}
	}
}
