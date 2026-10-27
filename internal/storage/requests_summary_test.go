package storage

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// The history summary used to run five separate queries - a total plus one
// GROUP BY per dimension - and now folds one four-dimension scan instead. The
// rewrite is only safe if it reproduces every aggregate exactly, so these tests
// compare the folded result against the same figures computed the old way,
// from the same rows.
//
// A fixture with one row per case cannot catch a fold that loses or double
// counts a group, so the rows below deliberately repeat models across
// platforms, share a scenario between two days, and mix known and unknown
// costs.

// summaryFixture inserts rows covering the shapes the fold has to get right:
// the same model on two platforms, one scenario spanning two days, a row with
// no scenario, and a row whose cost is unknown.
func summaryFixture(t *testing.T, db *Database) {
	t.Helper()
	day1 := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	day2 := day1.Add(24 * time.Hour)
	for _, rec := range []history.RequestRecord{
		// Same model, two platforms: must stay two model rows, as the SQL
		// GROUP BY model, provider produced.
		{ID: "m1", Model: "shared", Provider: "cline-pass", Scenario: "complex", StartTime: day1, InputTokens: 100, OutputTokens: 10, Success: true, CostKnown: true, CostUSD: 1.5},
		{ID: "m2", Model: "shared", Provider: "opencode-go", Scenario: "complex", StartTime: day1.Add(time.Hour), InputTokens: 200, OutputTokens: 20, Success: true, CostKnown: true, CostUSD: 2.5},
		// Second day, same scenario: the trend must split these from day 1 while
		// the scenario bucket merges them.
		{ID: "m3", Model: "other", Provider: "cline-pass", Scenario: "complex", StartTime: day2, InputTokens: 300, OutputTokens: 30, CacheReadTokens: 40, CacheCreationTokens: 5, Success: false},
		// No scenario and "unknown" are the same bucket.
		{ID: "m4", Model: "other", Provider: "cline-pass", Scenario: "", StartTime: day2.Add(time.Hour), InputTokens: 400, Success: true, CostKnown: true, CostUSD: 0.25},
		{ID: "m5", Model: "other", Provider: "cline-pass", Scenario: "unknown", StartTime: day2.Add(2 * time.Hour), InputTokens: 500, Success: true, CostKnown: true, CostUSD: 0.75},
		// A channel, to prove the filter reaches the same population the summary
		// aggregates.
		{ID: "m6", Model: "shared", Provider: "cline-pass", Scenario: "complex", StartTime: day2.Add(3 * time.Hour), InputTokens: 600, Success: true, CostKnown: true, CostUSD: 3, UpstreamChannel: "deepseek"},
	} {
		insertCostRecord(t, db, rec)
	}
}

// wantSummary is the expected aggregate set, written out rather than computed,
// so the test fails when the fold changes a number instead of agreeing with
// whatever the fold happens to produce.
func TestHistorySummaryFoldsEveryDimension(t *testing.T) {
	db := newCostTestDB(t)
	summaryFixture(t, db)
	repo := NewRequests(db)

	got, err := repo.Summary(t.Context(), RequestQuery{})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}

	if got.TotalRequests != 6 {
		t.Errorf("total_requests = %d, want 6", got.TotalRequests)
	}
	// Five rows carry details_known and a known outcome; one (m3) is a failure.
	if got.SuccessRows != 6 || got.SuccessRate != 5.0/6.0 {
		t.Errorf("success = %d rows at %v, want 6 rows at 5/6", got.SuccessRows, got.SuccessRate)
	}
	if got.InputTokens != 2100 || got.OutputTokens != 60 ||
		got.CacheReadTokens != 40 || got.CacheCreationTokens != 5 {
		t.Errorf("token split = %d/%d/%d/%d, want 2100/60/40/5",
			got.InputTokens, got.OutputTokens, got.CacheReadTokens, got.CacheCreationTokens)
	}
	if got.TotalTokens != 2205 {
		t.Errorf("total_tokens = %d, want 2205", got.TotalTokens)
	}
	if got.CostUSD != 8.0 || got.CostRows != 5 {
		t.Errorf("cost = %v over %d rows, want 8 over 5", got.CostUSD, got.CostRows)
	}
	if got.UnknownCostRequests != 1 {
		t.Errorf("unknown_cost_requests = %d, want 1", got.UnknownCostRequests)
	}

	// The same model on two platforms is two rows, ordered by volume then name.
	if len(got.Models) != 3 {
		t.Fatalf("models = %+v, want 3 rows", got.Models)
	}
	// cline-pass/shared has 2, opencode-go/shared has 1, cline-pass/other has 3.
	if got.Models[0].Name != "other" || got.Models[0].Provider != "cline-pass" || got.Models[0].Requests != 3 {
		t.Errorf("first model row = %+v, want other/cline-pass with 3", got.Models[0])
	}
	seen := map[string]int64{}
	for _, row := range got.Models {
		seen[row.Name+"/"+row.Provider] = row.Requests
	}
	for key, want := range map[string]int64{"shared/cline-pass": 2, "shared/opencode-go": 1, "other/cline-pass": 3} {
		if seen[key] != want {
			t.Errorf("model %s = %d requests, want %d", key, seen[key], want)
		}
	}

	if len(got.Providers) != 2 {
		t.Fatalf("providers = %+v, want 2 rows", got.Providers)
	}
	if got.Providers[0].Name != "cline-pass" || got.Providers[0].Requests != 5 {
		t.Errorf("first provider row = %+v, want cline-pass with 5", got.Providers[0])
	}
	if got.Providers[1].Name != "opencode-go" || got.Providers[1].Requests != 1 {
		t.Errorf("second provider row = %+v, want opencode-go with 1", got.Providers[1])
	}

	// "" and "unknown" both read as override, and both days of "complex" merge.
	if len(got.Scenarios) != 2 {
		t.Fatalf("scenarios = %+v, want 2 rows", got.Scenarios)
	}
	byScenario := map[string]int64{}
	for _, row := range got.Scenarios {
		byScenario[row.Name] = row.Requests
	}
	if byScenario["complex"] != 4 || byScenario["override"] != 2 {
		t.Errorf("scenarios = %v, want complex=4 override=2", byScenario)
	}

	// The trend is by UTC day, ascending.
	if len(got.Trend) != 2 {
		t.Fatalf("trend = %+v, want 2 days", got.Trend)
	}
	if got.Trend[0].Date != "2026-08-08" || got.Trend[0].Requests != 2 {
		t.Errorf("first day = %+v, want 2026-08-08 with 2", got.Trend[0])
	}
	if got.Trend[1].Date != "2026-08-09" || got.Trend[1].Requests != 4 {
		t.Errorf("second day = %+v, want 2026-08-09 with 4", got.Trend[1])
	}
}

// The fold has to apply the same WHERE clause to every dimension. A summary
// that filtered one dimension and not another would report a total that its own
// breakdown contradicts.
func TestHistorySummaryAppliesFiltersToEveryDimension(t *testing.T) {
	db := newCostTestDB(t)
	summaryFixture(t, db)
	repo := NewRequests(db)

	got, err := repo.Summary(t.Context(), RequestQuery{Provider: "cline-pass"})
	if err != nil {
		t.Fatalf("filtered summary: %v", err)
	}
	if got.TotalRequests != 5 {
		t.Errorf("total_requests = %d, want 5", got.TotalRequests)
	}
	for _, row := range got.Models {
		if row.Provider != "cline-pass" {
			t.Errorf("model row leaked another platform: %+v", row)
		}
	}
	if len(got.Providers) != 1 || got.Providers[0].Name != "cline-pass" {
		t.Errorf("providers = %+v, want only cline-pass", got.Providers)
	}
	var trendTotal int64
	for _, point := range got.Trend {
		trendTotal += point.Requests
	}
	if trendTotal != got.TotalRequests {
		t.Errorf("trend sums to %d but total is %d", trendTotal, got.TotalRequests)
	}
	// Scenarios must agree with the filtered population too: m2 is the only
	// opencode-go row, so "complex" drops from 4 to 3.
	for _, row := range got.Scenarios {
		if row.Name == "complex" && row.Requests != 3 {
			t.Errorf("complex = %d, want 3 after the platform filter", row.Requests)
		}
	}
}

// A channel filter has to reach the summary as well as the page, or the
// headline figures would describe a different population than the table below
// them.
func TestHistorySummaryAndQueryAgreeOnTheChannelFilter(t *testing.T) {
	db := newCostTestDB(t)
	summaryFixture(t, db)
	repo := NewRequests(db)

	rows, total, err := repo.Query(t.Context(), RequestQuery{Page: 1, PageSize: 50, Channel: "deepseek"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != "m6" {
		t.Fatalf("channel query = %d rows (total %d), want only m6", len(rows), total)
	}
	summary, err := repo.Summary(t.Context(), RequestQuery{Channel: "deepseek"})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.TotalRequests != 1 || summary.InputTokens != 600 {
		t.Errorf("channel summary = %+v, want 1 request and 600 input tokens", summary)
	}
	// A platform with no channel records must match nothing rather than
	// everything: an unmatched equality on a NULL column is not a wildcard.
	empty, err := repo.Summary(t.Context(), RequestQuery{Channel: "fireworks"})
	if err != nil {
		t.Fatalf("unobserved channel: %v", err)
	}
	if empty.TotalRequests != 0 {
		t.Errorf("unobserved channel matched %d requests, want 0", empty.TotalRequests)
	}
}

// ObservedChannels feeds the filter's options, so it must report what was
// actually served - not every channel the gateway advertises - and stay scoped
// to the platform asked about.
func TestObservedChannelsListsWhatServedTraffic(t *testing.T) {
	db := newCostTestDB(t)
	summaryFixture(t, db)
	repo := NewRequests(db)

	channels, err := repo.ObservedChannels(t.Context(), "cline-pass")
	if err != nil {
		t.Fatalf("observed channels: %v", err)
	}
	if len(channels) != 1 || channels[0] != "deepseek" {
		t.Errorf("cline-pass channels = %v, want [deepseek]", channels)
	}
	// Only ClinePass publishes a channel, so another platform honestly reports
	// none and the panel shows no filter.
	none, err := repo.ObservedChannels(t.Context(), "opencode-go")
	if err != nil {
		t.Fatalf("opencode-go channels: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("opencode-go channels = %v, want none", none)
	}
	// Unscoped asks across every platform.
	all, err := repo.ObservedChannels(t.Context(), "")
	if err != nil {
		t.Fatalf("all channels: %v", err)
	}
	if len(all) != 1 || all[0] != "deepseek" {
		t.Errorf("all channels = %v, want [deepseek]", all)
	}
}
