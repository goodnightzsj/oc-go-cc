package storage

import (
	"database/sql"
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
//
// The token counts stay inside the base context band, because the tier check
// runs on the whole prompt: one million of every category would total 3M and
// trip the 256K band, pricing at the long-context rates instead.
func TestCostForTokens_CacheWriteRate(t *testing.T) {
	// 50K of each category totals 150K of prompt, safely inside Qwen's 256K
	// band; four times this would cross it and change the expected numbers.
	const perCategory = 50_000

	// Qwen3.7 Plus base band: in 0.40, out 1.60, cache_read 0.04, cache_write 0.50.
	got := costForTokens("opencode-go", "qwen3.7-plus", perCategory, perCategory, perCategory, perCategory)
	want := (0.40 + 1.60 + 0.04 + 0.50) * 0.05
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("qwen3.7-plus cost = %v, want %v", got, want)
	}

	// DeepSeek V4 Flash has no cache_write price, so creation bills at input.
	got = costForTokens("opencode-go", "deepseek-v4-flash", perCategory, perCategory, perCategory, perCategory)
	want = (0.15 + 0.60 + 0.003 + 0.15) * 0.05
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("deepseek-v4-flash cost = %v, want %v", got, want)
	}
}

// A prompt that crosses a published context band bills at that band's rates.
// This is the difference the tier support exists for: before it, a 300K-token
// Qwen3.7 Plus turn was priced at the ≤256K rate and under-billed 3x.
func TestCostForTokens_ContextTierSwitches(t *testing.T) {
	// 300K input, no cache, 1K output: past the 256K band.
	in, out := int64(300_000), int64(1_000)
	got := costForTokens("opencode-go", "qwen3.7-plus", in, out, 0, 0)
	want := (float64(in)*1.20 + float64(out)*4.80) / 1e6
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("past-band cost = %v, want %v (1.20/4.80)", got, want)
	}
	// One token below the threshold stays on the base band.
	in = 256_000
	got = costForTokens("opencode-go", "qwen3.7-plus", in, out, 0, 0)
	want = (float64(in)*0.40 + float64(out)*1.60) / 1e6
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("at-threshold cost = %v, want %v (0.40/1.60)", got, want)
	}
	// The band is measured on the whole prompt, so cache reads count toward it.
	got = costForTokens("opencode-go", "qwen3.7-plus", 1_000, out, 300_000, 0)
	want = (1_000*1.20 + 300_000*0.12 + float64(out)*4.80) / 1e6
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("cache-inclusive band cost = %v, want %v", got, want)
	}
}

// TestCostForProviderTokensAt_UnpricedStaysUnknown covers a platform with no
// published prices and a model absent from a platform that has some. Both must
// report unknown rather than zero: a rate this proxy invented would be logged
// as a real cost, and a missing rate must never read as free usage.
func TestCostForProviderTokensAt_UnpricedStaysUnknown(t *testing.T) {
	at := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		provider string
		model    string
	}{
		{"platform publishes no rates", "opencode-zen", "qwen3.7-plus"},
		{"model absent from its platform's table", "commandcode", "totally-unknown-model"},
		{"unknown platform", "no-such-platform", "deepseek-v4-flash"},
	}
	for _, c := range cases {
		if cost, ok := costForProviderTokensAt(c.provider, c.model, 1_000, 1_000, 0, 0,
			sql.NullFloat64{}, sql.NullFloat64{}, at); ok {
			t.Errorf("%s: got cost=%v ok=true, want unknown", c.name, cost)
		}
	}
}

// TestModelBreakdownSumsToSummary is the regression guard for the bug where the
// per-model SQL priced only input+output at models-table rates while the summary
// used the seed rules, so the breakdown silently disagreed with the headline cost
// and every cached token was billed at the full input rate.
func TestModelBreakdownSumsToSummary(t *testing.T) {
	db := newCostTestDB(t)

	// Fixed off-peak start times: costForRecord prices at time.Now() when the
	// record has no StartTime, and deepseek rows double in the weekly peak
	// window — a clock-dependent test would fail depending on when it runs.
	offPeak := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) // Wednesday 12:00Z, deepseek ×1
	insertCostRecord(t, db, history.RequestRecord{
		ID: "r1", Model: "deepseek-v4-flash", Provider: "opencode-go",
		InputTokens: 4_000, CacheReadTokens: 96_000, OutputTokens: 2_000,
		Streaming: true, Success: true, StartTime: offPeak,
	})
	insertCostRecord(t, db, history.RequestRecord{
		ID: "r2", Model: "qwen3.7-plus", Provider: "opencode-go",
		InputTokens: 10_000, CacheReadTokens: 50_000, CacheCreationTokens: 20_000,
		OutputTokens: 5_000, Success: true, StartTime: offPeak,
	})
	insertCostRecord(t, db, history.RequestRecord{
		ID: "r3", Model: "deepseek-v4-flash", Provider: "opencode-go",
		InputTokens: 1_000, OutputTokens: 500, Success: false, StartTime: offPeak,
	})

	a := NewAnalytics(db)
	summary, err := a.TokenSummary(a.Window(30))
	if err != nil {
		t.Fatalf("GetTokenSummary: %v", err)
	}
	breakdown, err := a.ModelBreakdown(a.Window(30))
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

	// Independently: cached tokens must cost far less than the same volume of
	// raw input, which is what the old SQL path got wrong.
	wantDeepseek := costForTokens("opencode-go", "deepseek-v4-flash", 5_000, 2_500, 96_000, 0)
	wantQwen := costForTokens("opencode-go", "qwen3.7-plus", 10_000, 5_000, 50_000, 20_000)
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
	// Zen pricing comes from its exact catalog entry, not Go's seed rates.
	if _, err := db.DB().Exec(`INSERT INTO models (id, provider, name, cost_input_per_m, cost_output_per_m)
		VALUES ('opencode-zen/qwen3.7-plus', 'opencode-zen', 'qwen3.7-plus', 0.4, 1.6)`); err != nil {
		t.Fatal(err)
	}

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
	providers, err := a.ProviderBreakdown(a.Window(30))
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

	models, err := a.ModelBreakdown(a.Window(30))
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
	goProvider := byName["opencode-go"]
	if goProvider.InputTokens != 15_000 || goProvider.OutputTokens != 7_500 ||
		goProvider.CacheReadTokens != 146_000 || goProvider.CacheCreationTokens != 20_000 {
		t.Errorf("opencode-go token totals = %+v", goProvider)
	}

	// Fallback rate: one of two opencode-go requests had attempt > 1.
	if got := goProvider.FallbackRate; math.Abs(got-50.0) > 1e-9 {
		t.Errorf("opencode-go fallback rate = %v, want 50", got)
	}
	if got := byName["opencode-zen"].FallbackRate; got != 0 {
		t.Errorf("opencode-zen fallback rate = %v, want 0", got)
	}
}
