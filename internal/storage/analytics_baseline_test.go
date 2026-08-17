package storage

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// TestAnalyticsWindow_DaysAndExplicitRangeAgree pins the shared window
// construction both entry points now funnel through: "last N days" and the
// equivalent explicit range must aggregate the same rows into the same JSON, the
// baseline must clamp both, and a provider-corrected row (usage_trusted=1) must
// stay in the window even though its timestamp predates the baseline.
func TestAnalyticsWindow_DaysAndExplicitRangeAgree(t *testing.T) {
	db := newCostTestDB(t)

	baseline := time.Now().Add(-3 * time.Hour)
	for _, rec := range []history.RequestRecord{
		{
			ID: "corrected", Model: "deepseek-v4-flash", Provider: "opencode-go", Success: true,
			StartTime: baseline.Add(-time.Hour), InputTokens: 1_000, OutputTokens: 100,
		},
		{
			ID: "untrustworthy", Model: "deepseek-v4-flash", Provider: "opencode-go", Success: true,
			StartTime: baseline.Add(-time.Hour), InputTokens: 2_000, OutputTokens: 200,
		},
		{
			ID: "recent", Model: "qwen3.7-plus", Provider: "opencode-go", Success: true,
			StartTime: baseline.Add(time.Hour), InputTokens: 4_000, OutputTokens: 400,
		},
	} {
		insertCostRecord(t, db, rec)
	}
	// Only the provider sync marks a row trusted, so do it the same way here.
	if _, err := db.DB().Exec(`UPDATE requests SET usage_trusted = 1 WHERE id = ?`, "corrected"); err != nil {
		t.Fatalf("mark row trusted: %v", err)
	}

	a := &Analytics{db: db, baseline: baseline}

	now := time.Now()
	explicit, err := a.WindowBetween(now.AddDate(0, 0, -7), now.Add(time.Second))
	if err != nil {
		t.Fatalf("WindowBetween: %v", err)
	}

	// Everything the dashboard serves, in wire form, for one resolved window.
	snapshot := func(label string, window Window) string {
		t.Helper()
		summary, err := a.TokenSummary(window)
		if err != nil {
			t.Fatalf("%s summary: %v", label, err)
		}
		if summary.TotalRequests != 2 {
			t.Errorf("%s: requests = %d, want 2 (corrected pre-baseline row plus the recent one)",
				label, summary.TotalRequests)
		}
		if summary.InputTokens != 5_000 {
			t.Errorf("%s: input tokens = %d, want 5000 (untrustworthy pre-baseline row excluded)",
				label, summary.InputTokens)
		}
		if !summary.PeriodStart.Equal(baseline) {
			t.Errorf("%s: PeriodStart = %v, want the clamped baseline %v", label, summary.PeriodStart, baseline)
		}
		models, err := a.ModelBreakdown(window)
		if err != nil {
			t.Fatalf("%s models: %v", label, err)
		}
		providers, err := a.ProviderBreakdown(window)
		if err != nil {
			t.Fatalf("%s providers: %v", label, err)
		}
		scenarios, err := a.ScenarioBreakdown(window)
		if err != nil {
			t.Fatalf("%s scenarios: %v", label, err)
		}
		trend, err := a.TokenTrend(window, "day")
		if err != nil {
			t.Fatalf("%s trend: %v", label, err)
		}
		// The two windows are resolved microseconds apart, so their end instants
		// differ by construction; nothing else may.
		wire := *summary
		wire.PeriodEnd = time.Time{}
		blob, err := json.Marshal(map[string]any{
			"summary": wire, "models": models, "providers": providers,
			"scenarios": scenarios, "trend": trend,
		})
		if err != nil {
			t.Fatalf("%s marshal: %v", label, err)
		}
		return string(blob)
	}

	if days, between := snapshot("days", a.Window(7)), snapshot("explicit range", explicit); days != between {
		t.Errorf("days path and explicit range disagree:\n days = %s\nrange = %s", days, between)
	}
}

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
	fullSummary, err := full.TokenSummary(full.Window(30))
	if err != nil {
		t.Fatalf("GetTokenSummary (no baseline): %v", err)
	}
	if fullSummary.TotalRequests != 2 {
		t.Fatalf("without baseline: requests = %d, want 2", fullSummary.TotalRequests)
	}

	// Same package: construct with the baseline directly rather than via a
	// production setter that exists only for tests.
	scoped := &Analytics{db: db, baseline: cutoff}
	scopedSummary, err := scoped.TokenSummary(scoped.Window(30))
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
	models, err := scoped.ModelBreakdown(scoped.Window(30))
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

	providers, err := scoped.ProviderBreakdown(scoped.Window(30))
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

	a := &Analytics{db: db, baseline: time.Now().AddDate(0, 0, -90)} // older than the 7-day window

	summary, err := a.TokenSummary(a.Window(7))
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
	if got := NewAnalytics(db).baseline; !got.Equal(want) {
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
