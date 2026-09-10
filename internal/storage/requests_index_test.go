package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRequestsInstantSortIndex(t *testing.T) {
	for _, name := range []string{"new", "existing"} {
		t.Run(name, func(t *testing.T) {
			cfg := Config{DatabasePath: filepath.Join(t.TempDir(), "requests.db")}
			if name == "existing" {
				db, err := Open(cfg)
				if err != nil {
					t.Fatal(err)
				}
				// Model the preceding schema while retaining mixed-offset timestamps.
				_, err = db.DB().Exec(`
					DROP INDEX IF EXISTS idx_requests_start_instant;
					INSERT INTO requests (id, model, start_time, cost_usd, cost_source)
					VALUES ('older', 'synthetic-model', '2026-09-10T09:00:00+08:00', 0.5, 'provider'),
					       ('newer', 'synthetic-model', '2026-09-10T03:00:00Z', 1.5, 'provider');
				`)
				closeErr := db.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("prepare preceding schema: %v; close: %v", err, closeErr)
				}
			}
			for pass := 0; pass < 2; pass++ {
				db, err := Open(cfg)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				rows, err := db.DB().Query(`EXPLAIN QUERY PLAN
					SELECT id, model, start_time, cost_usd FROM requests
					ORDER BY julianday(start_time) DESC, id ASC LIMIT 50 OFFSET 0`)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = rows.Close() })
				var plan []string
				for rows.Next() {
					var id, parent, unused int
					var detail string
					if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
						t.Fatal(err)
					}
					plan = append(plan, detail)
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				if err := rows.Close(); err != nil {
					t.Fatal(err)
				}
				details := strings.Join(plan, "\n")
				if !strings.Contains(details, "idx_requests_start_instant") || strings.Contains(details, "TEMP B-TREE") {
					t.Errorf("pass %d: default history sort requires its instant index, got:\n%s", pass, details)
				}
				var oldIndexCount int
				if err := db.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_requests_start_time'`).Scan(&oldIndexCount); err != nil {
					t.Fatal(err)
				}
				if oldIndexCount != 1 {
					t.Error("the existing timestamp index must remain available")
				}
				if name == "existing" {
					var count int
					var cost float64
					var stamp string
					if err := db.DB().QueryRow(`SELECT COUNT(*), SUM(cost_usd), MAX(CASE WHEN id = 'older' THEN start_time END) FROM requests`).Scan(&count, &cost, &stamp); err != nil {
						t.Fatal(err)
					}
					if count != 2 || cost != 2 || stamp != "2026-09-10T09:00:00+08:00" {
						t.Errorf("index migration changed request rows: count=%d cost=%v stamp=%s", count, cost, stamp)
					}
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
