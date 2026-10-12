package storage

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// The upstream channel is a per-platform fact, so the column has to keep three
// states apart, not two:
//
//   - a channel was observed and recorded
//   - no channel was observed (platform does not publish one, or the row
//     predates the column)
//   - the column does not exist yet (a database from an older version)
//
// The last one is what the migration covers, and a fixture that only ever
// inserts rows cannot catch it: an unmigrated column would fail the INSERT and
// the test would pass on a fresh schema either way. So this opens a database,
// drops the column back to the old shape, and checks that startup re-adds it.
func TestUpstreamChannelSurvivesTheRoundTrip(t *testing.T) {
	db := newCostTestDB(t)
	repo := NewRequests(db)
	base := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

	if err := repo.Insert(history.RequestRecord{
		ID: "with", Model: "cline-pass/deepseek-v4.1-flash", Provider: "cline-pass",
		StartTime: base, Duration: time.Second, Success: true, DetailsKnown: true,
		UpstreamChannel: "openai-compatible-private",
	}); err != nil {
		t.Fatalf("insert with channel: %v", err)
	}
	if err := repo.Insert(history.RequestRecord{
		ID: "without", Model: "opencode-go/glm-5.2", Provider: "opencode-go",
		StartTime: base.Add(time.Minute), Duration: time.Second, Success: true, DetailsKnown: true,
	}); err != nil {
		t.Fatalf("insert without channel: %v", err)
	}

	records, _, err := repo.Query(RequestQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	byID := map[string]history.RequestRecord{}
	for _, rec := range records {
		byID[rec.ID] = rec
	}

	if got := byID["with"].UpstreamChannel; got != "openai-compatible-private" {
		t.Errorf("observed channel = %q, want it preserved", got)
	}
	// The absent case must come back empty rather than as the string "NULL" or
	// as some placeholder - the UI decides what to show by this being empty.
	if got := byID["without"].UpstreamChannel; got != "" {
		t.Errorf("unobserved channel = %q, want empty", got)
	}
}

// A database created before the column existed must gain it on startup, or
// every insert fails and history silently stops being written.
func TestUpstreamChannelColumnIsMigratedOnAnOldDatabase(t *testing.T) {
	db := newCostTestDB(t)

	// Reproduce the pre-column shape. SQLite can drop a column since 3.35, and
	// the point is that migrateColumns has to put it back.
	if _, err := db.DB().Exec(`ALTER TABLE requests DROP COLUMN upstream_channel`); err != nil {
		t.Skipf("this SQLite cannot drop columns (%v); the migration path is still covered by a fresh open", err)
	}
	if err := db.migrateColumns(t.Context()); err != nil {
		t.Fatalf("migrateColumns: %v", err)
	}

	repo := NewRequests(db)
	if err := repo.Insert(history.RequestRecord{
		ID: "after-migration", Model: "cline-pass/deepseek-v4.1-flash", Provider: "cline-pass",
		StartTime: time.Now(), Success: true, UpstreamChannel: "fireworks",
	}); err != nil {
		t.Fatalf("insert after migration: %v", err)
	}
	records, _, err := repo.Query(RequestQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(records) != 1 || records[0].UpstreamChannel != "fireworks" {
		t.Errorf("after migration channel = %+v, want it readable", records)
	}
}
