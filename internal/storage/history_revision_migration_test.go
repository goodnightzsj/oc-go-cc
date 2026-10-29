package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryRevisionWaitsForExistingWriter(t *testing.T) {
	for _, wal := range []bool{false, true} {
		name := "rollback"
		if wal {
			name = "wal"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history.db")
			db, err := Open(Config{DatabasePath: path, WALEnabled: wal})
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.DB().Exec(`
				DROP TRIGGER requests_revision_insert;
				DROP TRIGGER requests_revision_update;
				DROP TRIGGER requests_revision_delete;
			`); err != nil {
				t.Fatal(err)
			}
			writer, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			tx, err := writer.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.Exec(`UPDATE history_revision SET revision=41 WHERE id=1`); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- db.initHistoryRevision(ctx) }()
			// Hold a real writer longer than the former read-upgrade retry loop,
			// but within the configured busy_timeout; the migration must wait.
			select {
			case err := <-result:
				t.Fatalf("migration returned before the writer released its lock: %v", err)
			case <-time.After(1200 * time.Millisecond):
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := <-result; err != nil {
				t.Fatalf("migration after writer commit: %v", err)
			}
			if err := db.initHistoryRevision(ctx); err != nil {
				t.Fatalf("repeat migration: %v", err)
			}
			if _, err := db.DB().Exec(`INSERT INTO requests(id,model,start_time) VALUES('after-migration','m','2026-10-29')`); err != nil {
				t.Fatal(err)
			}
			var revision, triggers int
			if err := db.DB().QueryRow(`SELECT revision FROM history_revision WHERE id=1`).Scan(&revision); err != nil {
				t.Fatal(err)
			}
			if err := db.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='trigger' AND name IN ('requests_revision_insert','requests_revision_update','requests_revision_delete')`).Scan(&triggers); err != nil {
				t.Fatal(err)
			}
			if revision != 42 || triggers != 3 {
				t.Fatalf("revision=%d triggers=%d, want preserved revision plus one write and all triggers", revision, triggers)
			}
		})
	}
}

func TestHistoryRevisionFailureRollsBackAndReleasesConnection(t *testing.T) {
	for _, cancelDuringSQL := range []bool{false, true} {
		name := "statement_error"
		if cancelDuringSQL {
			name = "cancellation"
		}
		t.Run(name, func(t *testing.T) {
			db := newCostTestDB(t)
			ctx := t.Context()
			ddl := `CREATE TABLE revision_probe(n INTEGER); INSERT INTO missing_revision_table VALUES(1);`
			if cancelDuringSQL {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
				ddl = `CREATE TABLE revision_probe(n INTEGER);
					WITH RECURSIVE numbers(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM numbers WHERE n<100000000)
					INSERT INTO revision_probe SELECT n FROM numbers;`
			}
			if err := db.applyHistoryRevision(ctx, ddl); err == nil {
				t.Fatal("failed migration reported success")
			}
			var tables int
			if err := db.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='revision_probe'`).Scan(&tables); err != nil {
				t.Fatal(err)
			}
			if tables != 0 {
				t.Fatal("failed transaction left a partial schema")
			}
			if err := db.initHistoryRevision(t.Context()); err != nil {
				t.Fatalf("connection was not reusable after rollback: %v", err)
			}
		})
	}
}

func TestHistoryRevisionLockFailureReleasesConnection(t *testing.T) {
	for _, phase := range []string{"begin", "commit"} {
		t.Run(phase, func(t *testing.T) {
			db := newCostTestDB(t)
			if _, err := db.DB().Exec(`PRAGMA busy_timeout=25`); err != nil {
				t.Fatal(err)
			}
			blocker, err := sql.Open("sqlite", db.path)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Close()
			tx, err := blocker.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if phase == "begin" {
				_, err = tx.Exec(`UPDATE history_revision SET revision=revision+1 WHERE id=1`)
			} else {
				var count int
				err = tx.QueryRow(`SELECT count(*) FROM history_revision`).Scan(&count)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := db.applyHistoryRevision(t.Context(), `CREATE TABLE revision_probe(n INTEGER)`); err == nil {
				t.Fatalf("migration succeeded despite a blocked %s", phase)
			}
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			var tables int
			if err := db.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='revision_probe'`).Scan(&tables); err != nil {
				t.Fatal(err)
			}
			if tables != 0 {
				t.Fatal("failed migration left its table behind")
			}
			if err := db.initHistoryRevision(t.Context()); err != nil {
				t.Fatalf("retry after releasing blocker: %v", err)
			}
		})
	}
}
