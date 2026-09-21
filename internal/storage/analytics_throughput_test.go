package storage

import (
	"math"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// TestThroughputRate_UnknownWithoutDuration pins that "nothing measured" is
// reported as zero, which every caller renders as unknown. A fabricated 0 tok/s
// would read as a stalled platform.
func TestThroughputRate_UnknownWithoutDuration(t *testing.T) {
	if got := (throughputCounters{OutputTokens: 500}).rate(); got != 0 {
		t.Errorf("rate with no duration = %v, want 0 (unknown)", got)
	}
	if got := (throughputCounters{}.rate()); got != 0 {
		t.Errorf("rate with no counters = %v, want 0", got)
	}
	// 100 tokens over 2 seconds is 50 per second.
	if got := (throughputCounters{OutputTokens: 100, DurationMs: 2000}).rate(); got != 50 {
		t.Errorf("rate = %v, want 50", got)
	}
}

// TestModelBreakdownThroughputWeightsByDuration is the guard for the choice of
// SUM(output)/SUM(duration) over the mean of each request's own rate.
//
// The fixture is built so the two disagree loudly: a long fast generation and a
// long-running tiny one. Weighting by duration gives 1010 tokens / 101 s = 10
// tok/s; averaging the two rates would give (1000 + 0.1)/2 = 500 tok/s, which
// no caller would recognise as the throughput they experienced.
func TestModelBreakdownThroughputWeightsByDuration(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	insertCostRecord(t, db, history.RequestRecord{
		ID: "fast", Model: "glm-5.2", Provider: "opencode-go",
		OutputTokens: 1000, Duration: time.Second, Success: true, StartTime: at,
	})
	insertCostRecord(t, db, history.RequestRecord{
		ID: "slow", Model: "glm-5.2", Provider: "opencode-go",
		OutputTokens: 10, Duration: 100 * time.Second, Success: true, StartTime: at,
	})

	a := NewAnalytics(db)
	window := a.Window(30)
	rows, err := a.ModelBreakdown(window)
	if err != nil {
		t.Fatalf("ModelBreakdown: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	want := 1010.0 / 101.0
	if math.Abs(rows[0].TokensPerSecond-want) > 1e-9 {
		t.Errorf("model throughput = %v, want %v (ratio of sums)", rows[0].TokensPerSecond, want)
	}

	// The provider row aggregates the same requests and must report the same
	// figure, or the platform health line would contradict the model table.
	providers, err := a.ProviderBreakdown(window)
	if err != nil {
		t.Fatalf("ProviderBreakdown: %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("provider rows = %d, want 1", len(providers))
	}
	if math.Abs(providers[0].TokensPerSecond-want) > 1e-9 {
		t.Errorf("provider throughput = %v, want %v", providers[0].TokensPerSecond, want)
	}
}

// TestThroughputExcludesUnmeasuredRequests pins the population the rate is
// computed over. A failed attempt's tokens were never delivered end to end, and
// a row with no observed details has no timing to divide by; admitting either
// would let a single large failure set the reported speed for a healthy
// platform.
func TestThroughputExcludesUnmeasuredRequests(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	insertCostRecord(t, db, history.RequestRecord{
		ID: "ok", Model: "glm-5.2", Provider: "opencode-go",
		OutputTokens: 100, Duration: 10 * time.Second, Success: true, StartTime: at,
	})
	// Failed: billed tokens, but nothing was generated end to end.
	insertCostRecord(t, db, history.RequestRecord{
		ID: "failed", Model: "glm-5.2", Provider: "opencode-go",
		OutputTokens: 1_000_000, Duration: 10 * time.Second, Success: false, StartTime: at,
	})
	// Written the way the importer writes an official bill: details_known = 0,
	// because an imported row has usage but no locally observed timing. The
	// local Insert path hardcodes details_known = 1, so this row can only be
	// produced by raw SQL.
	if _, err := db.DB().Exec(`
		INSERT INTO requests (id, model, provider, start_time, duration_ms,
			output_tokens, details_known, usage_trusted, streaming, success, attempt)
		VALUES ('unobserved', 'glm-5.2', 'opencode-go', ?, 10000, 1000000, 0, 0, 1, 1, 1)`,
		at.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}

	a := NewAnalytics(db)
	rows, err := a.ModelBreakdown(a.Window(30))
	if err != nil {
		t.Fatalf("ModelBreakdown: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	// Only the successful, measured request counts: 100 tokens / 10s = 10.
	if math.Abs(rows[0].TokensPerSecond-10) > 1e-9 {
		t.Errorf("throughput = %v, want 10 (failures and unobserved rows excluded)", rows[0].TokensPerSecond)
	}
	if rows[0].Requests != 3 {
		t.Errorf("requests = %d, want 3 (all three rows are still counted)", rows[0].Requests)
	}
}

// TestThroughputIsUnknownWithoutMeasuredRequests covers the empty-denominator
// case end to end: the panel must be able to tell "no successful request was
// measured" from "the platform generated zero tokens per second".
func TestThroughputIsUnknownWithoutMeasuredRequests(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	insertCostRecord(t, db, history.RequestRecord{
		ID: "failed", Model: "glm-5.2", Provider: "opencode-go",
		OutputTokens: 500, Duration: 10 * time.Second, Success: false, StartTime: at,
	})

	rows, err := NewAnalytics(db).ModelBreakdown(NewAnalytics(db).Window(30))
	if err != nil {
		t.Fatalf("ModelBreakdown: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].TokensPerSecond != 0 {
		t.Errorf("throughput = %v, want 0 so the panel renders it as unknown", rows[0].TokensPerSecond)
	}
}
