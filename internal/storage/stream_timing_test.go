package storage

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestStreamTimingStoragePreservesUnknownAndZero(t *testing.T) {
	db := newCostTestDB(t)
	repo := NewRequests(db)
	zero, content, end := int64(0), int64(4000), int64(14000)
	for _, rec := range []history.RequestRecord{
		{ID: "measured", FirstEventMs: &zero, FirstContentMs: &content, MessageEndMs: &end},
		{ID: "legacy"},
	} {
		rec.Model = "synthetic"
		rec.StartTime = time.Now()
		rec.Duration = 16 * time.Second
		rec.Streaming = true
		rec.Success = true
		rec.OutputTokens = 600
		if err := repo.Insert(rec); err != nil {
			t.Fatal(err)
		}
	}
	rows, _, err := repo.Query(t.Context(), RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range rows {
		if rec.ID == "legacy" {
			if rec.FirstEventMs != nil || rec.FirstContentMs != nil || rec.MessageEndMs != nil {
				t.Fatal("unknown observations were fabricated")
			}
		} else if rec.FirstEventMs == nil || *rec.FirstEventMs != 0 || rec.FirstContentMs == nil || *rec.FirstContentMs != 4000 || rec.MessageEndMs == nil || *rec.MessageEndMs != 14000 {
			t.Fatalf("timing lost: %+v", rec)
		}
	}
}

func TestStreamTimingMigrationKeepsOldRowsUnknown(t *testing.T) {
	db := newCostTestDB(t)
	// A fixture with the historical schema, not a fresh database that already
	// has the new columns. Never skip a migration failure.
	for _, column := range []string{"first_event_ms", "first_content_ms", "message_end_ms"} {
		columns, err := db.tableColumns(t.Context(), "requests")
		if err != nil {
			t.Fatal(err)
		}
		if columns[column] {
			if _, err := db.DB().Exec("ALTER TABLE requests DROP COLUMN " + column); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.DB().Exec(`INSERT INTO requests (id,model,start_time,duration_ms,input_tokens,output_tokens) VALUES ('old','synthetic','2026-10-25T00:00:00Z',1000,10,20)`); err != nil {
		t.Fatal(err)
	}
	if err := db.migrateColumns(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := db.migrateColumns(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM requests WHERE id='old' AND first_event_ms IS NULL AND first_content_ms IS NULL AND message_end_ms IS NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("old timing: count=%d err=%v", count, err)
	}
	// The old projection and insert shape remain usable after upgrading.
	if _, err := db.DB().Exec(`INSERT INTO requests (id,model,start_time,duration_ms,input_tokens,output_tokens) VALUES ('old-writer','synthetic','2026-10-25T00:00:01Z',1000,10,20)`); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT SUM(output_tokens) FROM requests`).Scan(&count); err != nil || count != 40 {
		t.Fatalf("legacy access changed: count=%d err=%v", count, err)
	}
}
