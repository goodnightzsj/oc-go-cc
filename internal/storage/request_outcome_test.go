package storage

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestUnobservedUsageRetainsKnownOutcome(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	repo := NewRequests(db)
	for _, rec := range []history.RequestRecord{
		{ID: "success", Model: "model", Provider: "workbuddy", StartTime: at, Duration: time.Second, Streaming: true, Success: true, OutputTokens: 100},
		{ID: "truncated", Model: "model", Provider: "workbuddy", StartTime: at, Duration: 3 * time.Second, Streaming: true, UsageUnobserved: true},
	} {
		if err := repo.Insert(rec); err != nil {
			t.Fatal(err)
		}
	}
	// Provider imports have usage, but their request outcomes were not observed.
	if _, err := db.DB().Exec(`INSERT INTO requests (id, model, provider, start_time, duration_ms, details_known, streaming, success, input_tokens, output_tokens) VALUES ('import', 'model', 'workbuddy', ?, 0, 0, 0, 0, 5, 10)`, at.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	hs, err := repo.Summary(t.Context(), RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if hs.SuccessRows != 2 || hs.SuccessRate != 0.5 {
		t.Errorf("history known=%d rate=%g; want 2,0.5", hs.SuccessRows, hs.SuccessRate)
	}
	failed := false
	rows, count, err := repo.Query(t.Context(), RequestQuery{Success: &failed})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(rows) != 1 || rows[0].ID != "truncated" || rows[0].CostKnown || rows[0].DetailsKnown {
		t.Errorf("failed history: count=%d rows=%+v; want only unpriced truncated row", count, rows)
	}
	if len(rows) == 1 && !rows[0].OutcomeKnown {
		t.Error("record lost its observed outcome")
	}
	totals, err := repo.Totals()
	if err != nil {
		t.Fatal(err)
	}
	if totals.Received != 3 || totals.Success != 1 || totals.Failed != 1 || totals.Streamed != 2 {
		t.Errorf("totals=%+v", totals)
	}
	a := NewAnalytics(db)
	window, err := a.WindowBetween(at, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := a.Summary(t.Context(), window)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Summary.KnownRequests != 2 || snap.Summary.ErrorRequests != 1 || snap.Summary.SuccessRate != 0.5 {
		t.Errorf("analytics=%+v; want known=2 errors=1 rate=0.5", snap.Summary)
	}
	hourly, err := a.TokenTrend(t.Context(), window, "hour")
	if err != nil {
		t.Fatal(err)
	}
	if len(hourly) != 1 || hourly[0].ErrorRequests != 1 || hourly[0].KnownRequests != 2 {
		t.Errorf("hourly=%+v; want one failure in two observed outcomes", hourly)
	}
	perf, err := NewLatency(db).Snapshot(t.Context(), at)
	if err != nil {
		t.Fatal(err)
	}
	if perf.Failures["workbuddy/model"] != 1 {
		t.Errorf("performance failures=%v", perf.Failures)
	}
	if len(perf.Stats) != 1 || perf.Stats[0].Count != 2 {
		t.Errorf("measured durations must survive missing usage: %+v", perf.Stats)
	}
}

func TestOutcomeMigrationPreservesLegacyEvidence(t *testing.T) {
	db := newCostTestDB(t)
	if _, err := db.DB().Exec(`ALTER TABLE requests DROP COLUMN outcome_known`); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id                       string
		details, stream, success int
	}{
		{"local", 1, 0, 1}, {"truncated", 0, 1, 0}, {"import", 0, 0, 0},
	} {
		if _, err := db.DB().Exec(`INSERT INTO requests (id, model, start_time, duration_ms, input_tokens, output_tokens, details_known, streaming, success) VALUES (?, 'model', '2026-10-26T12:00:00Z', 0, 0, 0, ?, ?, ?)`, row.id, row.details, row.stream, row.success); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := db.migrateColumns(t.Context()); err != nil {
			t.Fatal(err)
		}
		rows, _, err := NewRequests(db).Query(t.Context(), RequestQuery{})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.OutcomeKnown != (row.ID != "import") {
				t.Errorf("legacy %s: outcome_known=%v", row.ID, row.OutcomeKnown)
			}
		}
	}
	// A new explicit observation flag overrides legacy inference.
	if _, err := db.DB().Exec(`UPDATE requests SET outcome_known = 0 WHERE id = 'truncated'`); err != nil {
		t.Fatal(err)
	}
	failed := false
	_, count, err := NewRequests(db).Query(t.Context(), RequestQuery{Success: &failed})
	if err != nil || count != 0 {
		t.Fatalf("explicit unknown: count=%d err=%v", count, err)
	}
}
