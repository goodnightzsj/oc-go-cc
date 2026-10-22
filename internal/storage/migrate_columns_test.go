package storage

import (
	"testing"
)

// The migration must decide what to add by asking the database, not by reading
// the driver's error text. Matching "duplicate column" worked against the
// wording modernc.org/sqlite happens to use; a release that reworded it would
// have made every ADD COLUMN look like a real failure, and no database would
// open. This runs the migration against a database whose columns all exist and
// requires it to complete with no error at all - which is only possible if the
// skip path runs.
func TestMigrateColumnsIsANoOpOnACurrentSchema(t *testing.T) {
	db := newCostTestDB(t)
	if err := db.migrateColumns(t.Context()); err != nil {
		t.Fatalf("migrating a current schema failed: %v", err)
	}
	// Twice, because a second run is what every startup does.
	if err := db.migrateColumns(t.Context()); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}
}

// A database created before these columns existed has to gain them, and the
// rows already in it keep NULL - the honest answer for a request that was never
// observed on that axis. A default would fabricate an observation.
func TestMigrateColumnsAddsMissingColumnsAndLeavesOldRowsNull(t *testing.T) {
	db := newCostTestDB(t)
	added := []string{"first_byte_ms", "reasoning_effort", "upstream_model", "upstream_channel"}
	for _, column := range added {
		if _, err := db.DB().Exec(`ALTER TABLE requests DROP COLUMN ` + column); err != nil {
			t.Skipf("this SQLite cannot drop columns (%v)", err)
		}
	}
	if _, err := db.DB().Exec(
		`INSERT INTO requests (id, model, provider, start_time, duration_ms) VALUES ('old','m','p','2026-01-01T00:00:00Z',100)`,
	); err != nil {
		t.Fatal(err)
	}

	if err := db.migrateColumns(t.Context()); err != nil {
		t.Fatalf("migration on an old database failed: %v", err)
	}

	var fb, re, um, uc any
	row := db.DB().QueryRow(
		`SELECT first_byte_ms, reasoning_effort, upstream_model, upstream_channel FROM requests WHERE id='old'`)
	if err := row.Scan(&fb, &re, &um, &uc); err != nil {
		t.Fatalf("the added columns are not readable: %v", err)
	}
	for name, value := range map[string]any{
		"first_byte_ms": fb, "reasoning_effort": re, "upstream_model": um, "upstream_channel": uc,
	} {
		if value != nil {
			t.Errorf("%s = %v for a pre-existing row, want NULL", name, value)
		}
	}
}

// The cost_source backfill is a full-table UPDATE. It used to run on every
// startup, scanning the whole history to change nothing once every row carried
// a source. It has to still run on a database that needs it, and be skipped
// afterwards.
func TestCostSourceBackfillRunsOnce(t *testing.T) {
	db := newCostTestDB(t)
	if _, err := db.DB().Exec(
		`INSERT INTO requests (id, model, provider, start_time, duration_ms, cost_usd, cost_source)
		 VALUES ('legacy','m','p','2026-01-01T00:00:00Z',100,0.5,NULL)`,
	); err != nil {
		t.Fatal(err)
	}
	if err := db.migrateColumns(t.Context()); err != nil {
		t.Fatalf("migration: %v", err)
	}

	var source string
	if err := db.DB().QueryRow(`SELECT COALESCE(cost_source,'') FROM requests WHERE id='legacy'`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "estimated" {
		t.Fatalf("a priced row without a source must be marked estimated, got %q", source)
	}

	// Re-introduce a row the backfill would match, then migrate again. The mark
	// means this run must not touch it - that is the difference between "runs
	// once" and "runs whenever it is idempotent".
	if _, err := db.DB().Exec(`UPDATE requests SET cost_source = NULL WHERE id='legacy'`); err != nil {
		t.Fatal(err)
	}
	if err := db.migrateColumns(t.Context()); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	var after string
	if err := db.DB().QueryRow(`SELECT COALESCE(cost_source,'') FROM requests WHERE id='legacy'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != "" {
		t.Errorf("the backfill ran again; the mark in schema_info did not hold")
	}
}
