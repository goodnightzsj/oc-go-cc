package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestPlatformDataCostsRespectProviderAndMissingRates(t *testing.T) {
	db := newCostTestDB(t)
	for _, price := range []struct {
		provider, model string
		input, output   any
	}{
		{"commandcode", "deepseek-v4-flash", 2.0, 8.0},
		{"openrouter", "deepseek-v4-flash", 3.0, 9.0},
		{"commandcode", "free-model", 0.0, 0.0},
		{"commandcode", "partial-model", 2.0, nil},
	} {
		if _, err := db.DB().Exec(`INSERT INTO models (id, provider, name, cost_input_per_m, cost_output_per_m) VALUES (?, ?, ?, ?, ?)`,
			price.provider+"/"+price.model, price.provider, price.model, price.input, price.output); err != nil {
			t.Fatal(err)
		}
	}
	peak := time.Date(2026, 9, 7, 1, 30, 0, 0, time.UTC)
	for _, tt := range []struct {
		name, provider, model string
		cacheRead, cacheWrite int
		known                 bool
		cost, multiplier      float64
	}{
		{"go seed", "opencode-go", "deepseek-v4-flash", 0, 0, true, 1.5, 2},
		{"legacy go seed", "", "deepseek-v4-flash", 0, 0, true, 1.5, 2},
		// CommandCode publishes its own rates, so they win over the catalog row
		// this test also inserts for the same model: 1M in at 0.15 + 1M out at
		// 0.60, doubled in the 01:30Z peak window.
		{"commandcode rate table", "commandcode", "deepseek-v4-flash", 0, 0, true, 1.5, 2},
		{"openrouter catalog", "openrouter", "deepseek-v4-flash", 0, 0, true, 12, 1},
		{"other provider has no catalog", "opencode-zen", "deepseek-v4-flash", 0, 0, false, 0, 1},
		{"unknown model", "commandcode", "unknown-model", 0, 0, false, 0, 1},
		{"explicit free catalog", "commandcode", "free-model", 0, 0, true, 0, 1},
		{"partial catalog", "commandcode", "partial-model", 0, 0, false, 0, 1},
		// CommandCode publishes a cache_read rate, so a cached turn stays
		// priced; its table has no cache_write rate for this model, so a cache
		// write bills at the input rate.
		{"priced cache read", "commandcode", "deepseek-v4-flash", 500, 0, true, 1.500003, 2},
		{"cache write billed as input", "commandcode", "deepseek-v4-flash", 0, 500, true, 1.50015, 2},
		// A catalog-priced platform has no cache rate at all, so those two
		// categories stay unknown rather than being billed as free usage.
		{"unpriced cache read", "openrouter", "deepseek-v4-flash", 500, 0, false, 0, 1},
		{"unpriced cache write", "openrouter", "deepseek-v4-flash", 0, 500, false, 0, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := history.RequestRecord{ID: tt.name, Provider: tt.provider, Model: tt.model, StartTime: peak,
				InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: tt.cacheRead,
				CacheCreationTokens: tt.cacheWrite, Success: true}
			if err := NewRequests(db).Insert(rec); err != nil {
				t.Fatal(err)
			}
			rows, _, err := NewRequests(db).Query(RequestQuery{Search: tt.name})
			if err != nil || len(rows) != 1 {
				t.Fatalf("Query = %v, %v", rows, err)
			}
			got := rows[0]
			if got.CostKnown != tt.known || math.Abs(got.CostUSD-tt.cost) > 1e-9 {
				t.Errorf("cost = (%v, %v), want (%v, %v)", got.CostKnown, got.CostUSD, tt.known, tt.cost)
			}
			if got.PeakMultiplier != tt.multiplier {
				t.Errorf("peak multiplier = %v, want %v", got.PeakMultiplier, tt.multiplier)
			}
		})
	}
	if _, err := db.DB().Exec(`UPDATE requests SET cost_usd = NULL, cost_source = NULL`); err != nil {
		t.Fatal(err)
	}
	if updated, err := db.BackfillRequestCosts(context.Background()); err != nil || updated != 7 {
		t.Fatalf("backfill provider catalog: updated=%d err=%v, want 7", updated, err)
	}
	for id, want := range map[string]sql.NullFloat64{
		"commandcode rate table":        {Float64: 1.5, Valid: true},
		"openrouter catalog":            {Float64: 12, Valid: true},
		"explicit free catalog":         {Valid: true},
		"priced cache read":             {Float64: 1.500003, Valid: true},
		"other provider has no catalog": {},
		"unpriced cache read":           {},
	} {
		var got sql.NullFloat64
		if err := db.DB().QueryRow(`SELECT cost_usd FROM requests WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("backfill %s cost=%v, want %v", id, got, want)
		}
	}
}

func TestPlatformDataAggregatesKeepKnownCosts(t *testing.T) {
	db := newCostTestDB(t)
	for _, rec := range []struct {
		id, provider                      string
		cost                              any
		known, success, attempt, duration int
	}{
		{"go-priced", "opencode-go", 9.5, 1, 1, 2, 100},
		{"go-unpriced", "opencode-go", nil, 1, 1, 1, 300},
		{"commandcode-priced", "commandcode", 17.0, 1, 0, 1, 900},
		{"commandcode-imported", "commandcode", nil, 0, 0, 2, 0},
	} {
		if _, err := db.DB().Exec(`INSERT INTO requests
			(id, provider, model, scenario, start_time, input_tokens, output_tokens, cost_usd, cost_source, details_known, success, attempt, duration_ms)
			VALUES (?, ?, 'deepseek-v4-flash', 'default', '2026-09-10T12:00:00Z', 1000000, 0, ?, 'provider', ?, ?, ?, ?)`,
			rec.id, rec.provider, rec.cost, rec.known, rec.success, rec.attempt, rec.duration); err != nil {
			t.Fatal(err)
		}
	}
	a := NewAnalytics(db)
	start := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	window, err := a.WindowBetween(start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	summary, err := a.TokenSummary(window)
	if err != nil {
		t.Fatal(err)
	}
	if summary.EstCostUSD != 26.5 || summary.UnknownCostRequests != 2 || summary.KnownRequests != 3 || math.Abs(summary.SuccessRate-2.0/3) > 1e-9 {
		t.Errorf("summary lost stored costs or known-detail denominator: %+v", summary)
	}
	models, err := a.ModelBreakdown(window)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Errorf("model rows = %d, want separate provider/model rows", len(models))
	}
	for _, row := range models {
		wantCost, wantLatency, wantKnown := 9.5, 200.0, int64(2)
		if row.Provider == "commandcode" {
			wantCost, wantLatency, wantKnown = 17, 900, 1
		}
		if row.Requests != 2 || row.KnownRequests != wantKnown || row.UnknownCostRequests != 1 || row.EstCostUSD != wantCost || row.AvgLatencyMs != wantLatency {
			t.Errorf("model aggregate combines providers or replaces stored costs: %+v", row)
		}
	}
	providers, err := a.ProviderBreakdown(window)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 2 {
		t.Fatalf("provider rows = %d, want 2", len(providers))
	}
	for _, row := range providers {
		wantCost, wantFallback, wantKnown := 9.5, 50.0, int64(2)
		if row.Provider == "commandcode" {
			wantCost, wantFallback, wantKnown = 17, 0, 1
		}
		if row.EstCostUSD != wantCost || row.FallbackRate != wantFallback || row.UnknownCostRequests != 1 || row.KnownRequests != wantKnown {
			t.Errorf("provider cost or known-detail denominator = %+v", row)
		}
	}
	scenarios, err := a.ScenarioBreakdown(window)
	if err != nil {
		t.Fatal(err)
	}
	if len(scenarios) != 1 {
		t.Fatalf("scenario rows = %d, want 1", len(scenarios))
	}
	if scenarios[0].EstCostUSD != 26.5 || scenarios[0].UnknownCostRequests != 2 || scenarios[0].KnownRequests != 3 || math.Abs(scenarios[0].SuccessRate-2.0/3) > 1e-9 {
		t.Errorf("scenario cost or known-detail denominator = %+v", scenarios)
	}
	historySummary, err := NewRequests(db).Summary(RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if historySummary.CostUSD != 26.5 || historySummary.CostRows != 2 || historySummary.UnknownCostRequests != 2 || len(historySummary.Models) != 2 {
		t.Errorf("history summary = %+v, want separate model/provider rows and stored cost sum", historySummary)
	}
	for _, row := range historySummary.Models {
		if row.Provider == "" || row.Requests != 2 || row.UnknownCostRequests != 1 {
			t.Errorf("history model loses provider or unknown costs: %+v", row)
		}
	}
	trend, err := a.TokenTrend(window, "day")
	if err != nil {
		t.Fatal(err)
	}
	if len(trend) != 1 {
		t.Fatalf("trend rows = %d, want 1", len(trend))
	}
	if trend[0].CostUSD != 26.5 || trend[0].UnknownCostRequests != 2 {
		t.Errorf("trend lost known costs or missing-price count: %+v", trend)
	}
	if len(historySummary.Trend) != 1 || historySummary.Trend[0].UnknownCostRequests != 2 {
		t.Errorf("history trend lost missing-price count: %+v", historySummary.Trend)
	}
	for _, value := range []any{summary, historySummary, scenarios[0], trend[0]} {
		blob, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]json.RawMessage
		if err := json.Unmarshal(blob, &wire); err != nil {
			t.Fatal(err)
		}
		if string(wire["unknown_cost_requests"]) != "2" {
			t.Errorf("wire cost completeness missing: %s", blob)
		}
	}
}

func TestPlatformDataBackfillUsesInstantsAndLeavesUnknownCosts(t *testing.T) {
	db := newCostTestDB(t)
	db.analyticsBaseline = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	for _, rec := range []struct {
		id, stamp, model string
		trusted          int
		cost             any
	}{
		{"before", "2026-09-10T19:00:00+08:00", "deepseek-v4-flash", 0, nil},
		{"after", "2026-09-10T13:00:00Z", "deepseek-v4-flash", 0, nil},
		{"boundary", "2026-09-10T20:00:00+08:00", "deepseek-v4-flash", 0, nil},
		{"trusted-old", "2026-09-10T10:00:00Z", "deepseek-v4-flash", 1, nil},
		{"unknown", "2026-09-10T13:00:00Z", "unknown-model", 0, nil},
		{"official", "2026-09-10T13:00:00Z", "deepseek-v4-flash", 0, 4.5},
		{"legacy-zero", "2026-09-10T13:00:00Z", "unknown-model", 0, 0.0},
	} {
		if _, err := db.DB().Exec(`INSERT INTO requests (id, provider, model, start_time, input_tokens, output_tokens, usage_trusted, cost_usd, cost_source)
			VALUES (?, 'opencode-go', ?, ?, 1000, 100, ?, ?, 'provider')`, rec.id, rec.model, rec.stamp, rec.trusted, rec.cost); err != nil {
			t.Fatal(err)
		}
	}
	updated, err := db.BackfillRequestCosts(context.Background())
	if err != nil || updated != 3 {
		t.Errorf("BackfillRequestCosts = %d, %v; want 3, nil", updated, err)
	}
	for _, id := range []string{"before", "unknown", "after", "boundary", "trusted-old", "official", "legacy-zero"} {
		var cost sql.NullFloat64
		if err := db.DB().QueryRow(`SELECT cost_usd FROM requests WHERE id = ?`, id).Scan(&cost); err != nil {
			t.Fatal(err)
		}
		wantKnown := id != "before" && id != "unknown"
		if cost.Valid != wantKnown {
			t.Errorf("%s cost = %v, want known=%v", id, cost, wantKnown)
		}
		if id == "official" && cost.Float64 != 4.5 || id == "legacy-zero" && cost.Float64 != 0 {
			t.Errorf("saved cost changed for %s: %v", id, cost)
		}
	}
	if updated, err := db.BackfillRequestCosts(context.Background()); err != nil || updated != 0 {
		t.Errorf("second backfill = %d, %v; want 0, nil", updated, err)
	}
}

func TestPlatformDataHistorySortsInstants(t *testing.T) {
	db := newCostTestDB(t)
	for _, rec := range []struct{ id, stamp string }{
		{"a", "2026-09-10T09:00:00+08:00"},
		{"b", "2026-09-10T03:00:00Z"},
		{"c", "2026-09-10T10:00:00+08:00"},
	} {
		if _, err := db.DB().Exec(`INSERT INTO requests (id, model, start_time, input_tokens, output_tokens, duration_ms) VALUES (?, 'same-model', ?, 0, 0, 0)`, rec.id, rec.stamp); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		field, order string
		ids          []string
	}{
		{"", "desc", []string{"b", "c", "a"}},
		{"", "asc", []string{"a", "c", "b"}},
		{"model", "asc", []string{"b", "c", "a"}},
	} {
		rows, _, err := NewRequests(db).Query(RequestQuery{SortBy: tt.field, SortOrder: tt.order})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		if !slices.Equal(ids, tt.ids) {
			t.Errorf("sort %q %q = %v, want %v", tt.field, tt.order, ids, tt.ids)
		}
	}
}

func TestPlatformDataPeakUsesUTCWeekday(t *testing.T) {
	west := time.FixedZone("UTC-05", -5*60*60)
	for _, tt := range []struct {
		stamp string
		want  float64
	}{
		{"2026-09-07T01:30:00Z", 2},
		{"2026-09-12T01:30:00Z", 1},
	} {
		instant, err := time.Parse(time.RFC3339, tt.stamp)
		if err != nil {
			t.Fatal(err)
		}
		for _, stamp := range []time.Time{instant, instant.In(west)} {
			if got := history.PeakMultiplier("deepseek-v4-flash", stamp); got != tt.want {
				t.Errorf("PeakMultiplier at %v = %v, want %v", stamp, got, tt.want)
			}
			for _, provider := range []struct {
				name  string
				peaks bool
			}{
				{"", true}, {"opencode-go", true},
				// CommandCode bills the same models on the same window.
				{"commandcode", true},
				// Platforms with no peak pricing at all.
				{"opencode-zen", false}, {"aws-bedrock", false}, {"openrouter", false},
			} {
				want := 1.0
				if provider.peaks {
					want = tt.want
				}
				if got := history.ProviderPeakMultiplier(provider.name, "deepseek-v4-flash", stamp); got != want {
					t.Errorf("%s peak at %v = %v, want %v", provider.name, stamp, got, want)
				}
			}
		}
	}
}

func TestPlatformDataUTCTrendBucketsAndWindow(t *testing.T) {
	db := newCostTestDB(t)
	for _, stamp := range []string{"2026-09-11T00:30:00+08:00", "2026-09-10T20:30:00Z"} {
		if _, err := db.DB().Exec(`INSERT INTO requests (id, model, start_time, input_tokens, output_tokens) VALUES (?, 'unknown-model', ?, 0, 0)`, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	a := NewAnalytics(db)
	start, err := time.Parse(time.RFC3339, "2026-09-10T08:00:00+08:00")
	if err != nil {
		t.Fatal(err)
	}
	window, err := a.WindowBetween(start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	summary, err := a.TokenSummary(window)
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalRequests != 2 || summary.PeriodStart.Location() != time.UTC || summary.PeriodEnd.Location() != time.UTC {
		t.Errorf("window must report UTC instants: %+v", summary)
	}
	for _, tt := range []struct {
		granularity string
		buckets     []string
	}{
		{"day", []string{"2026-09-10"}},
		{"hour", []string{"2026-09-10T16:00:00Z", "2026-09-10T20:00:00Z"}},
	} {
		points, err := a.TokenTrend(window, tt.granularity)
		if err != nil {
			t.Fatal(err)
		}
		var buckets []string
		for _, point := range points {
			buckets = append(buckets, point.Date)
		}
		if !slices.Equal(buckets, tt.buckets) {
			t.Errorf("%s buckets = %v, want %v", tt.granularity, buckets, tt.buckets)
		}
	}
}

func TestPlatformDataBackfillRollsBackFailedBatch(t *testing.T) {
	db := newCostTestDB(t)
	for _, id := range []string{"first", "reject"} {
		if _, err := db.DB().Exec(`INSERT INTO requests (id, provider, model, start_time, input_tokens, output_tokens)
			VALUES (?, 'opencode-go', 'deepseek-v4-flash', '2026-09-10T12:00:00Z', 1000, 100)`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.DB().Exec(`CREATE TRIGGER reject_estimate BEFORE UPDATE OF cost_usd ON requests
		WHEN NEW.id = 'reject' BEGIN SELECT RAISE(ABORT, 'synthetic write failure'); END`); err != nil {
		t.Fatal(err)
	}
	if updated, err := db.BackfillRequestCosts(context.Background()); err == nil || updated != 0 {
		t.Errorf("failed backfill reported committed changes: %d, %v", updated, err)
	}
	var known int64
	if err := db.DB().QueryRow(`SELECT COUNT(cost_usd) FROM requests`).Scan(&known); err != nil {
		t.Fatal(err)
	}
	if known != 0 {
		t.Errorf("failed backfill committed %d costs", known)
	}
}

func TestPlatformDataEmptyBreakdownsKeepArrayShape(t *testing.T) {
	a := NewAnalytics(newCostTestDB(t))
	window := a.Window(1)
	providers, err := a.ProviderBreakdown(window)
	if err != nil {
		t.Fatal(err)
	}
	scenarios, err := a.ScenarioBreakdown(window)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{providers, scenarios} {
		body, err := json.Marshal(value)
		if err != nil || string(body) != "[]" {
			t.Errorf("empty breakdown = %s, %v; want []", body, err)
		}
	}
}

func TestPlatformDataUnknownOutcomesDoNotBecomeFailures(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "only-unknown", true: "mixed"}[mixed], func(t *testing.T) {
			db := newCostTestDB(t)
			if _, err := db.DB().Exec(`INSERT INTO requests
				(id, provider, model, scenario, start_time, details_known, success, attempt)
				VALUES ('legacy-null', 'commandcode', 'shared-model', 'default', '2026-09-10T12:00:00Z', 1, NULL, 2),
				       ('imported', 'commandcode', 'shared-model', 'default', '2026-09-10T12:00:00Z', 0, 1, 2)`); err != nil {
				t.Fatal(err)
			}
			var wantKnown, wantErrors int64
			var wantRate, wantFallback float64
			if mixed {
				if _, err := db.DB().Exec(`INSERT INTO requests
					(id, provider, model, scenario, start_time, details_known, success, attempt)
					VALUES ('success', 'commandcode', 'shared-model', 'default', '2026-09-10T12:00:00Z', 1, 1, 1),
					       ('failure', 'commandcode', 'shared-model', 'default', '2026-09-10T12:00:00Z', 1, 0, 2)`); err != nil {
					t.Fatal(err)
				}
				wantKnown, wantErrors, wantRate, wantFallback = 2, 1, 0.5, 50
			}
			a := NewAnalytics(db)
			start := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
			window, err := a.WindowBetween(start, start.Add(24*time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			summary, err := a.TokenSummary(window)
			if err != nil {
				t.Fatal(err)
			}
			if summary.TotalRequests != wantKnown+2 || summary.KnownRequests != wantKnown || summary.ErrorRequests != wantErrors || summary.SuccessRate != wantRate {
				t.Errorf("summary must exclude unknown outcomes from rates: %+v", summary)
			}
			models, err := a.ModelBreakdown(window)
			if err != nil || len(models) != 1 {
				t.Fatalf("model breakdown = %+v, %v", models, err)
			}
			if models[0].KnownRequests != wantKnown || models[0].SuccessRate != wantRate {
				t.Errorf("model outcomes = %+v", models[0])
			}
			scenarios, err := a.ScenarioBreakdown(window)
			if err != nil || len(scenarios) != 1 {
				t.Fatalf("scenario breakdown = %+v, %v", scenarios, err)
			}
			if scenarios[0].KnownRequests != wantKnown || scenarios[0].SuccessRate != wantRate {
				t.Errorf("scenario outcomes = %+v", scenarios[0])
			}
			providers, err := a.ProviderBreakdown(window)
			if err != nil || len(providers) != 1 {
				t.Fatalf("provider breakdown = %+v, %v", providers, err)
			}
			if providers[0].KnownRequests != wantKnown || providers[0].FallbackRate != wantFallback {
				t.Errorf("provider outcomes = %+v", providers[0])
			}
			trend, err := a.TokenTrend(window, "day")
			if err != nil || len(trend) != 1 {
				t.Fatalf("trend = %+v, %v", trend, err)
			}
			if trend[0].KnownRequests != wantKnown || trend[0].ErrorRequests != wantErrors {
				t.Errorf("trend outcomes = %+v", trend[0])
			}
			history, err := NewRequests(db).Summary(RequestQuery{})
			if err != nil {
				t.Fatal(err)
			}
			if history.TotalRequests != wantKnown+2 || history.SuccessRows != wantKnown || history.SuccessRate != wantRate {
				t.Errorf("history outcomes = %+v", history)
			}
			success, failure, err := NewLatency(db).GetSuccessCounts(time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if success["commandcode/shared-model"] != wantKnown-wantErrors || failure["commandcode/shared-model"] != wantErrors {
				t.Errorf("latency outcomes: success=%v failure=%v", success, failure)
			}
		})
	}
}
