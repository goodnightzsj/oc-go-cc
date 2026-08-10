package storage

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func newCostTestDB(t *testing.T) *Database {
	t.Helper()
	db, err := Open(Config{
		DatabasePath: filepath.Join(t.TempDir(), "cost.db"),
		WALEnabled:   false,
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertCostRecord(t *testing.T, db *Database, rec history.RequestRecord) {
	t.Helper()
	if rec.ID == "" {
		rec.ID = t.Name() + time.Now().Format("150405.000000000")
	}
	if rec.StartTime.IsZero() {
		rec.StartTime = time.Now().Add(-time.Minute)
	}
	if err := NewRequests(db).Insert(rec); err != nil {
		t.Fatalf("insert record: %v", err)
	}
}

// TestCostForTokens_CacheWriteRate pins the OpenCode billing rules: cache reads
// use the cheap cache_read rate, and cache creation uses cache_write when the
// model publishes one (Qwen3.7 Plus writes at $0.50 vs $0.40 input) or the input
// rate otherwise (DeepSeek V4 Flash).
func TestCostForTokens_CacheWriteRate(t *testing.T) {
	const million = 1_000_000

	// Qwen3.7 Plus: in 0.40, out 1.60, cache_read 0.04, cache_write 0.50.
	got := costForTokens("qwen3.7-plus", million, million, million, million, 0, 0)
	want := 0.40 + 1.60 + 0.04 + 0.50
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("qwen3.7-plus cost = %v, want %v", got, want)
	}

	// DeepSeek V4 Flash has no cache_write price, so creation bills at input.
	got = costForTokens("deepseek-v4-flash", million, million, million, million, 0, 0)
	want = 0.14 + 0.28 + 0.0028 + 0.14
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("deepseek-v4-flash cost = %v, want %v", got, want)
	}
}

// TestCostForTokens_ModelsTableFallback covers a model with no seed rule: the
// models-table rates price input/output, and cache reads must not be billed at
// the full input rate.
func TestCostForTokens_ModelsTableFallback(t *testing.T) {
	const million = 1_000_000
	got := costForTokens("totally-unknown-model", million, million, million, 0, 2.0, 8.0)
	if want := 2.0 + 8.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("fallback cost = %v, want %v (cache reads must stay unpriced)", got, want)
	}
	if got := costForTokens("totally-unknown-model", million, million, 0, 0, 0, 0); got != 0 {
		t.Errorf("unknown model with no rates = %v, want 0", got)
	}
}

// TestModelBreakdownSumsToSummary is the regression guard for the bug where the
// per-model SQL priced only input+output at models-table rates while the summary
// used the seed rules, so the breakdown silently disagreed with the headline cost
// and every cached token was billed at the full input rate.
func TestModelBreakdownSumsToSummary(t *testing.T) {
	db := newCostTestDB(t)

	insertCostRecord(t, db, history.RequestRecord{
		ID: "r1", Model: "deepseek-v4-flash", Provider: "opencode-go",
		InputTokens: 4_000, CacheReadTokens: 96_000, OutputTokens: 2_000,
		Streaming: true, Success: true,
	})
	insertCostRecord(t, db, history.RequestRecord{
		ID: "r2", Model: "qwen3.7-plus", Provider: "opencode-go",
		InputTokens: 10_000, CacheReadTokens: 50_000, CacheCreationTokens: 20_000,
		OutputTokens: 5_000, Success: true,
	})
	insertCostRecord(t, db, history.RequestRecord{
		ID: "r3", Model: "deepseek-v4-flash", Provider: "opencode-go",
		InputTokens: 1_000, OutputTokens: 500, Success: false,
	})

	a := NewAnalytics(db)
	summary, err := a.GetTokenSummary(30)
	if err != nil {
		t.Fatalf("GetTokenSummary: %v", err)
	}
	breakdown, err := a.GetModelBreakdown(30)
	if err != nil {
		t.Fatalf("GetModelBreakdown: %v", err)
	}
	if len(breakdown) != 2 {
		t.Fatalf("breakdown rows = %d, want 2", len(breakdown))
	}

	var sum float64
	for _, mb := range breakdown {
		if mb.EstCostUSD <= 0 {
			t.Errorf("%s: est cost = %v, want > 0", mb.Model, mb.EstCostUSD)
		}
		sum += mb.EstCostUSD
	}
	if math.Abs(sum-summary.EstCostUSD) > 1e-9 {
		t.Errorf("breakdown sum %v != summary %v", sum, summary.EstCostUSD)
	}
	if summary.ProviderCostRows != 0 || summary.EstimatedCostRows != 3 {
		t.Errorf("cost provenance = provider:%d estimated:%d, want 0/3", summary.ProviderCostRows, summary.EstimatedCostRows)
	}

	// Independently: cached tokens must cost far less than the same volume of
	// raw input, which is what the old SQL path got wrong.
	wantDeepseek := costForTokens("deepseek-v4-flash", 5_000, 2_500, 96_000, 0, 0, 0)
	wantQwen := costForTokens("qwen3.7-plus", 10_000, 5_000, 50_000, 20_000, 0, 0)
	if math.Abs(summary.EstCostUSD-(wantDeepseek+wantQwen)) > 1e-9 {
		t.Errorf("summary %v, want %v", summary.EstCostUSD, wantDeepseek+wantQwen)
	}
}

// TestGetProviderBreakdown_CostIsRealAndMatchesModels pins the fix for the
// shipped placeholder that hardcoded provider cost to zero. Prices are per-model,
// so a provider row must accumulate its models' costs — and the provider totals
// must agree with the model breakdown for the same window.
func TestGetProviderBreakdown_CostIsRealAndMatchesModels(t *testing.T) {
	db := newCostTestDB(t)

	// Two models under one provider, one model under another.
	insertCostRecord(t, db, history.RequestRecord{
		ID: "p1", Model: "qwen3.7-plus", Provider: "opencode-go", Success: true, Attempt: 1,
		InputTokens: 10_000, OutputTokens: 5_000, CacheReadTokens: 50_000, CacheCreationTokens: 20_000,
	})
	insertCostRecord(t, db, history.RequestRecord{
		ID: "p2", Model: "deepseek-v4-flash", Provider: "opencode-go", Success: true, Attempt: 2,
		InputTokens: 5_000, OutputTokens: 2_500, CacheReadTokens: 96_000,
	})
	insertCostRecord(t, db, history.RequestRecord{
		ID: "p3", Model: "qwen3.7-plus", Provider: "opencode-zen", Success: true, Attempt: 1,
		InputTokens: 1_000, OutputTokens: 500,
	})

	a := NewAnalytics(db)
	providers, err := a.GetProviderBreakdown(30)
	if err != nil {
		t.Fatalf("GetProviderBreakdown: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("provider rows = %d, want 2", len(providers))
	}

	var providerSum float64
	byName := map[string]ProviderBreakdown{}
	for _, pb := range providers {
		if pb.EstCostUSD <= 0 {
			t.Errorf("%s: est cost = %v, want > 0 (placeholder regression)", pb.Provider, pb.EstCostUSD)
		}
		providerSum += pb.EstCostUSD
		byName[pb.Provider] = pb
	}

	models, err := a.GetModelBreakdown(30)
	if err != nil {
		t.Fatalf("GetModelBreakdown: %v", err)
	}
	var modelSum float64
	for _, mb := range models {
		modelSum += mb.EstCostUSD
	}
	if math.Abs(providerSum-modelSum) > 1e-9 {
		t.Errorf("provider total %v != model total %v", providerSum, modelSum)
	}

	// Ordering is by request count, so the two-request provider comes first.
	if providers[0].Provider != "opencode-go" {
		t.Errorf("rows[0] = %q, want opencode-go (ordered by requests)", providers[0].Provider)
	}

	// Fallback rate: one of two opencode-go requests had attempt > 1.
	if got := byName["opencode-go"].FallbackRate; math.Abs(got-50.0) > 1e-9 {
		t.Errorf("opencode-go fallback rate = %v, want 50", got)
	}
	if got := byName["opencode-zen"].FallbackRate; got != 0 {
		t.Errorf("opencode-zen fallback rate = %v, want 0", got)
	}
}
