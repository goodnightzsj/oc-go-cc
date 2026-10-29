package storage

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// Opening the same old database from several connections at once used to fail:
// each one saw the column missing and all of them ran the same ALTER, so the
// losers got "duplicate column" and the open aborted. The schema was fine.
//
// This reproduces the shape rather than the timing: a schema missing the newer
// columns, opened concurrently.
func TestConcurrentOpenOfAnOldDatabaseAllSucceeds(t *testing.T) {
	for _, wal := range []bool{false, true} {
		t.Run(fmt.Sprintf("wal=%v", wal), func(t *testing.T) {
			checkConcurrentOpenOfAnOldDatabase(t, wal)
		})
	}
}

func checkConcurrentOpenOfAnOldDatabase(t *testing.T, wal bool) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.db")

	// Build an old-shaped database: requests with none of the added columns.
	old, err := Open(Config{DatabasePath: path, WALEnabled: wal})
	if err != nil {
		t.Fatalf("seed open: %v", err)
	}
	if _, err := old.DB().Exec(`DROP TABLE IF EXISTS requests`); err != nil {
		t.Fatal(err)
	}
	// Only the newer columns are absent; the base shape (and its indexes) stay,
	// so what fails is the race, not a schema too old to open at all.
	if _, err := old.DB().Exec(`CREATE TABLE requests (
		id TEXT PRIMARY KEY,
		model TEXT NOT NULL,
		provider TEXT,
		scenario TEXT,
		start_time TIMESTAMP NOT NULL,
		duration_ms INTEGER,
		input_tokens INTEGER,
		output_tokens INTEGER,
		streaming INTEGER,
		success INTEGER,
		error_msg TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	const openers = 16
	var wg sync.WaitGroup
	errs := make([]error, openers)
	dbs := make([]*Database, openers)
	start := make(chan struct{})
	for i := 0; i < openers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			dbs[i], errs[i] = Open(Config{DatabasePath: path, WALEnabled: wal})
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("opener %d failed: %v", i, err)
		}
	}
	for _, db := range dbs {
		if db != nil {
			_ = db.Close()
		}
	}

	// The migration still had to happen exactly once, correctly.
	final, err := Open(Config{DatabasePath: path, WALEnabled: wal})
	if err != nil {
		t.Fatalf("final open: %v", err)
	}
	defer func() { _ = final.Close() }()
	cols, err := final.tableColumns(t.Context(), "requests")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"cost_usd", "cost_source", "first_byte_ms", "reasoning_effort", "upstream_model"} {
		if !cols[want] {
			t.Errorf("column %q missing after concurrent migration: %v", want, cols)
		}
	}
	if dup := len(cols); dup == 0 {
		t.Fatal(fmt.Sprintf("no columns read: %v", cols))
	}
}
