package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// TestAnalyticsBaseline_ExcludesUntrustworthyRows covers the real incident this
// mechanism exists for: an earlier build recorded whole prompts as fresh input
// because the upstream cache fields were never parsed, so those rows price a
// cache hit at the full input rate. The split cannot be reconstructed after the
// fact, so the baseline drops them from every aggregate.
func TestAnalyticsBaseline_ExcludesUntrustworthyRows(t *testing.T) {
	db := newCostTestDB(t)

	cutoff := time.Now().Add(-6 * time.Hour)

	// Before the cutoff: 1M prompt tokens all booked as fresh input.
	insertCostRecord(t, db, history.RequestRecord{
		ID: "old", Model: "deepseek-v4-flash", Provider: "opencode-go", Success: true,
		StartTime:   cutoff.Add(-2 * time.Hour),
		InputTokens: 1_000_000, OutputTokens: 1_000,
	})
	// After the cutoff: the same prompt volume, correctly attributed to cache.
	insertCostRecord(t, db, history.RequestRecord{
		ID: "new", Model: "deepseek-v4-flash", Provider: "opencode-go", Success: true,
		StartTime:       cutoff.Add(2 * time.Hour),
		CacheReadTokens: 1_000_000, OutputTokens: 1_000,
	})

	full := NewAnalytics(db)
	fullSummary, err := full.GetTokenSummary(30)
	if err != nil {
		t.Fatalf("GetTokenSummary (no baseline): %v", err)
	}
	if fullSummary.TotalRequests != 2 {
		t.Fatalf("without baseline: requests = %d, want 2", fullSummary.TotalRequests)
	}

	scoped := NewAnalytics(db)
	scoped.SetBaseline(cutoff)
	scopedSummary, err := scoped.GetTokenSummary(30)
	if err != nil {
		t.Fatalf("GetTokenSummary (baseline): %v", err)
	}

	if scopedSummary.TotalRequests != 1 {
		t.Errorf("with baseline: requests = %d, want 1", scopedSummary.TotalRequests)
	}
	if scopedSummary.InputTokens != 0 {
		t.Errorf("with baseline: input tokens = %d, want 0 (pre-cutoff row excluded)", scopedSummary.InputTokens)
	}
	if scopedSummary.CacheReadTokens != 1_000_000 {
		t.Errorf("with baseline: cache read = %d, want 1000000", scopedSummary.CacheReadTokens)
	}

	// The whole point: dropping the mis-attributed row collapses the cost,
	// because cache reads bill at $0.0028/M against $0.14/M for fresh input.
	if scopedSummary.EstCostUSD >= fullSummary.EstCostUSD {
		t.Errorf("baseline cost %v should be below full-history cost %v",
			scopedSummary.EstCostUSD, fullSummary.EstCostUSD)
	}

	// PeriodStart must report the clamped window, otherwise the dashboard would
	// claim to cover 30 days while only querying the trustworthy tail.
	if !scopedSummary.PeriodStart.Equal(cutoff) {
		t.Errorf("PeriodStart = %v, want the baseline %v", scopedSummary.PeriodStart, cutoff)
	}

	// Every aggregate must honour the same cutoff, not just the summary.
	models, err := scoped.GetModelBreakdown(30)
	if err != nil {
		t.Fatalf("GetModelBreakdown: %v", err)
	}
	var modelRequests int64
	for _, mb := range models {
		modelRequests += mb.Requests
	}
	if modelRequests != 1 {
		t.Errorf("model breakdown requests = %d, want 1", modelRequests)
	}

	providers, err := scoped.GetProviderBreakdown(30)
	if err != nil {
		t.Fatalf("GetProviderBreakdown: %v", err)
	}
	var providerRequests int64
	for _, pb := range providers {
		providerRequests += pb.Requests
	}
	if providerRequests != 1 {
		t.Errorf("provider breakdown requests = %d, want 1", providerRequests)
	}
}

// TestAnalyticsBaseline_DoesNotWidenWindow pins that the baseline only ever
// narrows the window. A cutoff older than the requested range must not pull in
// data from outside "last N days".
func TestAnalyticsBaseline_DoesNotWidenWindow(t *testing.T) {
	db := newCostTestDB(t)

	insertCostRecord(t, db, history.RequestRecord{
		ID: "ancient", Model: "deepseek-v4-flash", Provider: "opencode-go", Success: true,
		StartTime:   time.Now().AddDate(0, 0, -20),
		InputTokens: 1_000, OutputTokens: 100,
	})
	insertCostRecord(t, db, history.RequestRecord{
		ID: "recent", Model: "deepseek-v4-flash", Provider: "opencode-go", Success: true,
		StartTime:   time.Now().Add(-time.Hour),
		InputTokens: 1_000, OutputTokens: 100,
	})

	a := NewAnalytics(db)
	a.SetBaseline(time.Now().AddDate(0, 0, -90)) // older than the 7-day window

	summary, err := a.GetTokenSummary(7)
	if err != nil {
		t.Fatalf("GetTokenSummary: %v", err)
	}
	if summary.TotalRequests != 1 {
		t.Errorf("requests = %d, want 1 (the 20-day-old row is outside the 7-day window)",
			summary.TotalRequests)
	}
}

// TestOpen_AnalyticsBaselineConfig covers the wiring: a configured timestamp is
// parsed once at startup and inherited by NewAnalytics, and a malformed value
// fails loudly instead of silently disabling the cutoff.
func TestOpen_AnalyticsBaselineConfig(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(Config{
		DatabasePath:      filepath.Join(dir, "ok.db"),
		AnalyticsBaseline: "2026-08-06T12:07:00Z",
	})
	if err != nil {
		t.Fatalf("Open with valid baseline: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	want := time.Date(2026, 8, 6, 12, 7, 0, 0, time.UTC)
	if got := db.AnalyticsBaseline(); !got.Equal(want) {
		t.Errorf("Database baseline = %v, want %v", got, want)
	}
	if got := NewAnalytics(db).Baseline(); !got.Equal(want) {
		t.Errorf("NewAnalytics did not inherit the baseline: got %v, want %v", got, want)
	}

	if _, err := Open(Config{
		DatabasePath:      filepath.Join(dir, "bad.db"),
		AnalyticsBaseline: "not-a-timestamp",
	}); err == nil {
		t.Error("Open accepted a malformed analytics_baseline, want an error")
	}

	plain, err := Open(Config{DatabasePath: filepath.Join(dir, "plain.db")})
	if err != nil {
		t.Fatalf("Open without baseline: %v", err)
	}
	t.Cleanup(func() { _ = plain.Close() })
	if !plain.AnalyticsBaseline().IsZero() {
		t.Errorf("empty config should leave the baseline unset, got %v", plain.AnalyticsBaseline())
	}
}
