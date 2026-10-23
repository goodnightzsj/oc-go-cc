package storage

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// The dashboard's analytics tab used to run five separate GROUP BY queries -
// one per panel - and each scanned the whole window. They are now folded from a
// single grouped scan.
//
// The risk in that rewrite is not performance, it is agreement: a fold that
// computes one dimension's aggregates slightly differently from the query it
// replaced produces a dashboard whose panels contradict each other, with no
// error anywhere. These tests pin the fold against hand-computed values and
// across dimensions.

func analyticsFixture(t *testing.T, db *Database) time.Time {
	t.Helper()
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	records := []history.RequestRecord{
		// Two successful models on one platform, one of them a fallback.
		{ID: "a1", Model: "alpha", Provider: "opencode-go", StartTime: at,
			InputTokens: 100, OutputTokens: 40, CacheReadTokens: 60, CacheCreationTokens: 10,
			Duration: 2 * time.Second, Success: true, Attempt: 1},
		{ID: "a2", Model: "alpha", Provider: "opencode-go", StartTime: at.Add(time.Hour),
			InputTokens: 200, OutputTokens: 60, Duration: 4 * time.Second, Success: true, Attempt: 2},
		{ID: "b1", Model: "beta", Provider: "cline-pass", StartTime: at.Add(2 * time.Hour),
			InputTokens: 300, OutputTokens: 100, Duration: 5 * time.Second, Success: true, Attempt: 1},
		// A failure: counted in requests, excluded from success and throughput.
		{ID: "c1", Model: "beta", Provider: "cline-pass", StartTime: at.Add(3 * time.Hour),
			InputTokens: 10, OutputTokens: 999, Duration: 5 * time.Second, Success: false, Attempt: 1},
	}
	for _, rec := range records {
		insertCostRecord(t, db, rec)
	}
	return at
}

func TestAnalyticsSnapshotAgreesAcrossDimensions(t *testing.T) {
	db := newCostTestDB(t)
	at := analyticsFixture(t, db)
	a := NewAnalytics(db)
	window, err := a.WindowBetween(at, at.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := a.Summary(window)
	if err != nil {
		t.Fatal(err)
	}

	// The summary is the total of every request in the window.
	if snap.Summary.TotalRequests != 4 {
		t.Fatalf("total requests = %d, want 4", snap.Summary.TotalRequests)
	}
	if snap.Summary.KnownRequests != 4 || snap.Summary.ErrorRequests != 1 {
		t.Errorf("known=%d errors=%d, want 4 and 1",
			snap.Summary.KnownRequests, snap.Summary.ErrorRequests)
	}
	if snap.Summary.InputTokens != 610 || snap.Summary.OutputTokens != 1199 {
		t.Errorf("input=%d output=%d, want 610 and 1199",
			snap.Summary.InputTokens, snap.Summary.OutputTokens)
	}
	if snap.Summary.SuccessRate != 0.75 {
		t.Errorf("success rate = %v, want 0.75", snap.Summary.SuccessRate)
	}

	// Each dimension must add up to the same totals. A fold that dropped a row
	// from one dimension only would show here.
	sum := func(n int, pick func(int) int64) int64 {
		var total int64
		for i := 0; i < n; i++ {
			total += pick(i)
		}
		return total
	}
	models := snap.Models
	if got := sum(len(models), func(i int) int64 { return models[i].Requests }); got != 4 {
		t.Errorf("model requests sum = %d, want 4", got)
	}
	if got := sum(len(models), func(i int) int64 { return models[i].InputTokens }); got != 610 {
		t.Errorf("model input sum = %d, want 610", got)
	}
	providers := snap.Providers
	if got := sum(len(providers), func(i int) int64 { return providers[i].Requests }); got != 4 {
		t.Errorf("provider requests sum = %d, want 4", got)
	}
	if got := sum(len(providers), func(i int) int64 { return providers[i].InputTokens }); got != 610 {
		t.Errorf("provider input sum = %d, want 610", got)
	}
	if len(snap.Trend) != 1 || snap.Trend[0].Requests != 4 || snap.Trend[0].InputTokens != 610 {
		t.Errorf("daily trend = %+v, want one bucket of 4 requests / 610 input", snap.Trend)
	}

	// A model row and the summary must not disagree about cost either.
	var modelCost float64
	for _, m := range models {
		modelCost += m.EstCostUSD
	}
	if diff := modelCost - snap.Summary.EstCostUSD; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("model cost sum %v != summary %v", modelCost, snap.Summary.EstCostUSD)
	}
}

// The per-dimension rates were computed under conditions that the old queries
// spelled out separately. Folding them into one scan must not widen any of them
// - a failure's output counted as throughput would inflate the platform's
// reported speed, which is exactly the figure an operator uses to judge it.
func TestAnalyticsSnapshotRatesUseTheirOwnPopulations(t *testing.T) {
	db := newCostTestDB(t)
	at := analyticsFixture(t, db)
	a := NewAnalytics(db)
	window, err := a.WindowBetween(at, at.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := a.Summary(window)
	if err != nil {
		t.Fatal(err)
	}

	byModel := map[string]ModelBreakdown{}
	for _, m := range snap.Models {
		byModel[m.Model] = m
	}
	alpha := byModel["alpha"]
	// 100 output tokens over 6s: the ratio of sums, not the mean of rates.
	if want := 100.0 / 6.0; alpha.TokensPerSecond < want-1e-9 || alpha.TokensPerSecond > want+1e-9 {
		t.Errorf("alpha throughput = %v, want %v", alpha.TokensPerSecond, want)
	}
	if alpha.SuccessRate != 1 {
		t.Errorf("alpha success = %v, want 1", alpha.SuccessRate)
	}
	// 4s + 2s over two measured requests.
	if want := 3000.0; alpha.AvgLatencyMs != want {
		t.Errorf("alpha avg latency = %v, want %v", alpha.AvgLatencyMs, want)
	}

	beta := byModel["beta"]
	// Only the successful request contributes: the failure's 999 output tokens
	// must be excluded or beta would look four times faster than it was.
	if want := 100.0 / 5.0; beta.TokensPerSecond < want-1e-9 || beta.TokensPerSecond > want+1e-9 {
		t.Errorf("beta throughput = %v, want %v (failure must be excluded)", beta.TokensPerSecond, want)
	}
	if beta.SuccessRate != 0.5 {
		t.Errorf("beta success = %v, want 0.5", beta.SuccessRate)
	}
	// Latency, by contrast, measures both attempts: a failure still took 5s.
	if beta.AvgLatencyMs != 5000 {
		t.Errorf("beta avg latency = %v, want 5000", beta.AvgLatencyMs)
	}

	// Fallback rate counts only the requests whose outcome is known, and only
	// one of the four went through more than one attempt.
	var openCodeGo ProviderBreakdown
	for _, p := range snap.Providers {
		if p.Provider == "opencode-go" {
			openCodeGo = p
		}
	}
	if want := 50.0; openCodeGo.FallbackRate != want {
		t.Errorf("opencode-go fallback rate = %v, want %v", openCodeGo.FallbackRate, want)
	}
}

// The hourly trend is a separate query because no panel asks for it by default.
// It must still agree with the daily one, which is computed the other way.
func TestHourlyTrendAgreesWithTheDailySnapshot(t *testing.T) {
	db := newCostTestDB(t)
	at := analyticsFixture(t, db)
	a := NewAnalytics(db)
	window, err := a.WindowBetween(at.Truncate(time.Hour), at.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	hourly, err := a.TokenTrend(window, "hour")
	if err != nil {
		t.Fatal(err)
	}
	daily, err := a.TokenTrend(window, "day")
	if err != nil {
		t.Fatal(err)
	}
	if len(hourly) != 4 {
		t.Fatalf("hourly buckets = %d, want 4", len(hourly))
	}
	var requests, input, output int64
	var cost float64
	for _, point := range hourly {
		if len(point.Date) != len("2006-01-02T15:04:05Z") {
			t.Errorf("hourly bucket key %q is not an RFC3339 hour", point.Date)
		}
		requests += point.Requests
		input += point.InputTokens
		output += point.OutputTokens
		cost += point.CostUSD
	}
	if len(daily) != 1 {
		t.Fatalf("daily buckets = %d, want 1", len(daily))
	}
	if daily[0].Requests != requests || daily[0].InputTokens != input ||
		daily[0].OutputTokens != output || daily[0].CostUSD != cost {
		t.Errorf("daily %+v disagrees with the hourly sum (%d/%d/%d/%v)",
			daily[0], requests, input, output, cost)
	}
}

// A cached snapshot must not outlive the rows it describes, and must not be
// recomputed when nothing changed. Both halves of the cache contract.
func TestAnalyticsSnapshotCacheFollowsTheTable(t *testing.T) {
	db := newCostTestDB(t)
	at := analyticsFixture(t, db)
	a := NewAnalytics(db)
	window, err := a.WindowBetween(at, at.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	first, err := a.Summary(window)
	if err != nil {
		t.Fatal(err)
	}
	if first.Summary.TotalRequests != 4 {
		t.Fatalf("initial total = %d", first.Summary.TotalRequests)
	}
	repeated, err := a.Summary(window)
	if err != nil {
		t.Fatal(err)
	}
	if repeated != first {
		t.Error("an unchanged table must reuse the cached snapshot")
	}

	insertCostRecord(t, db, history.RequestRecord{
		ID: "a3", Model: "alpha", Provider: "opencode-go", StartTime: at.Add(4 * time.Hour),
		InputTokens: 50, Success: true, Attempt: 1,
	})
	after, err := a.Summary(window)
	if err != nil {
		t.Fatal(err)
	}
	if after.Summary.TotalRequests != 5 || after.Summary.InputTokens != 660 {
		t.Errorf("after insert: requests=%d input=%d, want 5 and 660 (cache did not notice)",
			after.Summary.TotalRequests, after.Summary.InputTokens)
	}
	if after == first {
		t.Error("a committed write must invalidate the snapshot")
	}
}

// Two windows must not share one entry. The cache is keyed by the resolved
// window, and a key that omitted the provider would serve an unfiltered answer
// under a filtered heading - silently, with every number well-formed.
func TestAnalyticsSnapshotCacheSeparatesWindows(t *testing.T) {
	db := newCostTestDB(t)
	at := analyticsFixture(t, db)
	a := NewAnalytics(db)
	all, err := a.WindowBetween(at, at.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	clineOnly, err := a.WindowBetween(at, at.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	everything, err := a.Summary(all)
	if err != nil {
		t.Fatal(err)
	}
	if everything.Summary.TotalRequests != 4 {
		t.Fatalf("unfiltered total = %d, want 4", everything.Summary.TotalRequests)
	}
	filtered, err := a.Summary(clineOnly.ForProvider("cline-pass"))
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Summary.TotalRequests != 2 {
		t.Errorf("provider filter returned %d requests, want 2 (cache leaked across windows)",
			filtered.Summary.TotalRequests)
	}
	if len(filtered.Models) != 1 || filtered.Models[0].Model != "beta" {
		t.Errorf("filtered models = %+v, want only beta", filtered.Models)
	}
	// And a different range must not reuse it either.
	shorter, err := a.WindowBetween(at, at.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	narrow, err := a.Summary(shorter)
	if err != nil {
		t.Fatal(err)
	}
	if narrow.Summary.TotalRequests >= everything.Summary.TotalRequests {
		t.Errorf("narrow window total = %d, want fewer than %d",
			narrow.Summary.TotalRequests, everything.Summary.TotalRequests)
	}
}
