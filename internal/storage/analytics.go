package storage

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

// Analytics provides aggregated metrics for the dashboard.
type Analytics struct {
	db *Database

	// baseline optionally excludes requests recorded before it. Rows written by
	// an earlier build can carry an unusable token split — most importantly a
	// prompt that was billed entirely as fresh input because the upstream cache
	// fields were never parsed. Such a row cannot be repaired after the fact
	// (the hit/miss breakdown is simply absent), so the only honest options are
	// to drop it or to exclude it from the window. Zero means "include all".
	baseline time.Time
}

// NewAnalytics creates a new Analytics store. It inherits the database's
// configured analytics baseline, so callers get the trustworthy window by
// default without having to know the cutoff exists.
func NewAnalytics(db *Database) *Analytics {
	return &Analytics{db: db, baseline: db.AnalyticsBaseline()}
}

// Window is a resolved analytics time range. Build one with Analytics.Window or
// Analytics.WindowBetween and hand the same value to every aggregate, so one
// dashboard request reports every panel over exactly the same range.
//
// requested is the range the caller asked for; trusted is that start pushed
// forward to the trust baseline. Both travel together because rows corrected in
// place carry usage_trusted=1 and stay in usage/cost analytics even when their
// timestamp predates trusted.
type Window struct {
	requested time.Time
	trusted   time.Time
	end       time.Time
	provider  string
}

// ForProvider returns a copy scoped to one canonical provider. Empty includes all
// providers, including legacy rows whose provider is unknown.
func (w Window) ForProvider(provider string) Window {
	w.provider = provider
	return w
}

// cacheKey identifies the aggregate this window produces. Every field the scan
// reads must appear here, or two different windows would share one entry.
func (w Window) cacheKey() string {
	return strings.Join([]string{
		w.requested.UTC().Format(time.RFC3339Nano),
		w.trusted.UTC().Format(time.RFC3339Nano),
		w.end.UTC().Format(time.RFC3339Nano),
		w.provider,
	}, "\x1f")
}

// Window returns the window covering the last N days. Non-positive days means 30.
//
// The window is snapped to whole minutes, which is what lets the snapshot cache
// engage: the overview polls a rolling range every three seconds, and an
// unsnapped `now` produced a new window - and therefore a new cache key - on
// every poll, so the most frequently requested aggregate in the dashboard was
// also the only one that never reused an answer.
//
// Snapping is a property of the window rather than of the cache, so a cached
// answer and a fresh one describe exactly the same rows. On a 7-day range the
// shift moves the boundary by under a minute at each edge.
func (a *Analytics) Window(days int) Window {
	if days <= 0 {
		days = 30
	}
	now := time.Now().UTC().Truncate(time.Minute)
	// SQLite stores request timestamps at millisecond precision. Leave a small
	// inclusive tolerance so a request inserted immediately before aggregation
	// cannot round to the half-open window's upper boundary.
	return a.window(now.AddDate(0, 0, -days), now.Add(time.Minute))
}

// WindowBetween returns the window for an explicit half-open range.
func (a *Analytics) WindowBetween(start, end time.Time) (Window, error) {
	if start.IsZero() || end.IsZero() || !start.Before(end) {
		return Window{}, errors.New("analytics start must be before end")
	}
	return a.window(start, end), nil
}

// window applies the trust baseline to the requested start. The baseline only
// ever narrows the window: a cutoff older than the request is ignored.
func (a *Analytics) window(start, end time.Time) Window {
	trusted := start
	if !a.baseline.IsZero() && a.baseline.After(trusted) {
		trusted = a.baseline
	}
	return Window{requested: start.UTC(), trusted: trusted.UTC(), end: end.UTC()}
}

// TokenSummary holds high-level token and request metrics for a time window.
type TokenSummary struct {
	TotalRequests       int64     `json:"total_requests"`
	KnownRequests       int64     `json:"known_requests"`
	ErrorRequests       int64     `json:"error_requests"`
	InputTokens         int64     `json:"input_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	CacheReadTokens     int64     `json:"cache_read_tokens"`
	CacheCreationTokens int64     `json:"cache_creation_tokens"`
	SuccessRate         float64   `json:"success_rate"` // 0-1
	EstCostUSD          float64   `json:"est_cost_usd"`
	UnknownCostRequests int64     `json:"unknown_cost_requests"`
	PeriodStart         time.Time `json:"period_start"`
	PeriodEnd           time.Time `json:"period_end"`
}

// ModelBreakdown holds per-model usage and performance stats.
type ModelBreakdown struct {
	Model               string  `json:"model"`
	Provider            string  `json:"provider"`
	Requests            int64   `json:"requests"`
	KnownRequests       int64   `json:"known_requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	SuccessRate         float64 `json:"success_rate"`
	TokensPerSecond     float64 `json:"tokens_per_second"`
	EstCostUSD          float64 `json:"est_cost_usd"` // sum of known stored request costs
	UnknownCostRequests int64   `json:"unknown_cost_requests"`
}

// throughputCounters carries the two sums TokensPerSecond is derived from.
//
// The rate is SUM(output)/SUM(duration), not the mean of each request's own
// rate: averaging ratios weights a 200-token reply the same as a 20K-token one,
// so a run of short requests would drag the reported figure below what the
// platform actually sustained. Totals keep the long generations' weight.
type throughputCounters struct {
	OutputTokens int64
	DurationMs   int64
}

// rate converts the counters to tokens per second of wall time. Zero duration
// means nothing measurable was in the window, which is unknown rather than
// zero throughput, so the caller reports it as such.
//
// This measures the whole request, including time-to-first-token; without a
// first-token timestamp the generation phase cannot be isolated, so the label
// must say "per second" and not "generation speed".
func (c throughputCounters) rate() float64 {
	if c.DurationMs <= 0 {
		return 0
	}
	return float64(c.OutputTokens) / (float64(c.DurationMs) / 1000.0)
}

// ProviderBreakdown holds per-provider aggregates.
//
// SuccessRate and AvgLatencyMs mirror ModelBreakdown's definitions so a
// provider row and the model rows beneath it agree: success counts only
// requests whose outcome is known, and latency averages only requests with a
// measured duration. Both are zero when nothing qualifying is in the window,
// which the panel renders as unknown rather than as 0%.
type ProviderBreakdown struct {
	Provider            string  `json:"provider"`
	Requests            int64   `json:"requests"`
	KnownRequests       int64   `json:"known_requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	SuccessRate         float64 `json:"success_rate"`
	TokensPerSecond     float64 `json:"tokens_per_second"`
	FallbackRate        float64 `json:"fallback_rate"` // % of known requests that were fallbacks
	EstCostUSD          float64 `json:"est_cost_usd"`
	UnknownCostRequests int64   `json:"unknown_cost_requests"`
}

// ScenarioBreakdown holds proxy-owned request aggregates. Scenarios are not
// available in the OpenCode account export, so this remains a local drill-down.
type ScenarioBreakdown struct {
	Scenario            string  `json:"scenario"`
	Requests            int64   `json:"requests"`
	KnownRequests       int64   `json:"known_requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	SuccessRate         float64 `json:"success_rate"`
	EstCostUSD          float64 `json:"est_cost_usd"`
	UnknownCostRequests int64   `json:"unknown_cost_requests"`
}

// DailyTokenPoint is a single day in the token trend.
type DailyTokenPoint struct {
	Date                string  `json:"date"` // UTC: YYYY-MM-DD, or RFC3339 for hourly buckets
	Requests            int64   `json:"requests"`
	KnownRequests       int64   `json:"known_requests"`
	ErrorRequests       int64   `json:"error_requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CostUSD             float64 `json:"cost_usd"`
	UnknownCostRequests int64   `json:"unknown_cost_requests"`
}

// AnalyticsSnapshot is every aggregate for one window, produced by a single
// scan of the request table.
//
// The dashboard polls the summary and the trend in parallel every three
// seconds, and each used to be its own full scan - five passes over a table
// that only grows, on a pure-Go SQLite driver where the scan and not the
// aggregation is what costs. Grouping once by every dimension the panels need,
// then folding in Go, makes one pass serve all of them. Measured on a 32K-row
// window: five separate scans cost 212ms, the single grouped pass 60ms.
//
// The trend here is daily. An hourly trend is a different, narrower query
// (`HourlyTrend`), because grouping 32K rows into 689 hour buckets costs about
// 45ms on its own and most views never ask for it.
//
// The fields are shared between callers of one cached snapshot. Callers must
// treat them as read-only.
type AnalyticsSnapshot struct {
	Summary   *TokenSummary
	Models    []ModelBreakdown
	Providers []ProviderBreakdown
	Scenarios []ScenarioBreakdown
	Trend     []DailyTokenPoint
}

// Summary returns every aggregate for the window. It reuses the previous answer
// while the request table has not changed, which is the normal case for a poll
// that runs every three seconds against a table growing by a few rows a minute.
func (a *Analytics) Summary(window Window) (*AnalyticsSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	revision, err := a.db.historyRevision(ctx)
	if err != nil {
		return nil, err
	}
	key := window.cacheKey()
	if cached, ok := a.db.cachedAnalytics(key, revision); ok {
		return cached, nil
	}
	snapshot, err := a.snapshot(ctx, window)
	if err != nil {
		return nil, err
	}
	a.db.storeAnalytics(key, revision, snapshot)
	return snapshot, nil
}

// analyticBucket accumulates one group's aggregates. It is a plain struct rather
// than the wire types so the same fold can feed all five views.
type analyticBucket struct {
	requests      int64
	known         int64
	errors        int64
	successes     int64
	input         int64
	output        int64
	cacheRead     int64
	cacheCreation int64
	cost          float64
	costRows      int64
	fallbacks     int64
	latencySum    int64
	latencyCount  int64
	tpTokens      int64
	tpDuration    int64
}

func (b *analyticBucket) add(o analyticBucket) {
	b.requests += o.requests
	b.known += o.known
	b.errors += o.errors
	b.successes += o.successes
	b.input += o.input
	b.output += o.output
	b.cacheRead += o.cacheRead
	b.cacheCreation += o.cacheCreation
	b.cost += o.cost
	b.costRows += o.costRows
	b.fallbacks += o.fallbacks
	b.latencySum += o.latencySum
	b.latencyCount += o.latencyCount
	b.tpTokens += o.tpTokens
	b.tpDuration += o.tpDuration
}

// successRate is zero when no request in the group reported an outcome, which
// every panel renders as unknown rather than as 0%.
func (b analyticBucket) successRate() float64 {
	if b.known <= 0 {
		return 0
	}
	return float64(b.successes) / float64(b.known)
}

func (b analyticBucket) avgLatencyMs() float64 {
	if b.latencyCount <= 0 {
		return 0
	}
	return float64(b.latencySum) / float64(b.latencyCount)
}

func (b analyticBucket) tokensPerSecond() float64 {
	return throughputCounters{OutputTokens: b.tpTokens, DurationMs: b.tpDuration}.rate()
}

// Two conditions are measured per row rather than per window. Both are spelled
// once here because the latency panel and the throughput column must not drift
// apart: a request whose duration was never observed contributes to neither.
const (
	measuredLatency = `r.details_known = 1 AND r.duration_ms > 0 ` +
		`AND julianday(r.start_time) >= julianday(?)`
	measuredThroughput = `r.details_known = 1 AND r.success = 1 ` +
		`AND r.duration_ms > 0 AND r.output_tokens > 0`
)

// snapshot runs the one scan every aggregate is folded from.
func (a *Analytics) snapshot(ctx context.Context, window Window) (*AnalyticsSnapshot, error) {
	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT r.model,
		       COALESCE(r.provider, '') AS provider_label,
		       COALESCE(r.provider, 'unknown') AS provider_key,
		       `+scenarioDimension+`,
		       DATE(r.start_time) AS day,
		       COUNT(*),
		       COUNT(CASE WHEN r.details_known = 1 AND r.success IN (0, 1) THEN 1 END),
		       COALESCE(SUM(CASE WHEN r.details_known = 1 AND r.success = 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN r.details_known = 1 AND r.success = 1 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(r.input_tokens), 0),
		       COALESCE(SUM(r.output_tokens), 0),
		       COALESCE(SUM(r.cache_read_tokens), 0),
		       COALESCE(SUM(r.cache_creation_tokens), 0),
		       COALESCE(SUM(r.cost_usd), 0),
		       COUNT(r.cost_usd),
		       COALESCE(SUM(CASE WHEN r.details_known = 1 AND r.success IN (0, 1)
		                         THEN COALESCE(r.attempt, 1) > 1 END), 0),
		       COALESCE(SUM(CASE WHEN `+measuredLatency+` THEN r.duration_ms ELSE 0 END), 0),
		       COUNT(CASE WHEN `+measuredLatency+` THEN 1 END),
		       COALESCE(SUM(CASE WHEN `+measuredThroughput+` THEN r.output_tokens ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN `+measuredThroughput+` THEN r.duration_ms ELSE 0 END), 0)
		FROM requests r
		WHERE julianday(r.start_time) >= julianday(?)
		  AND julianday(r.start_time) < julianday(?)
		  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
		  AND (? = '' OR r.provider = ?)
		GROUP BY 1, 2, 4, 5
	`, window.trusted.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano),
		window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano),
		window.trusted.Format(time.RFC3339Nano), window.provider, window.provider)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	models := map[modelGroup]*analyticBucket{}
	providers := map[string]*analyticBucket{}
	scenarios := map[string]*analyticBucket{}
	days := map[string]*analyticBucket{}
	var total analyticBucket

	bucketFor := func(m map[string]*analyticBucket, key string) *analyticBucket {
		if current, ok := m[key]; ok {
			return current
		}
		created := &analyticBucket{}
		m[key] = created
		return created
	}

	for rows.Next() {
		var group modelGroup
		var providerKey, scenario, day string
		var b analyticBucket
		if err := rows.Scan(&group.model, &group.provider, &providerKey, &scenario, &day,
			&b.requests, &b.known, &b.errors, &b.successes,
			&b.input, &b.output, &b.cacheRead, &b.cacheCreation,
			&b.cost, &b.costRows, &b.fallbacks,
			&b.latencySum, &b.latencyCount, &b.tpTokens, &b.tpDuration); err != nil {
			return nil, err
		}
		total.add(b)
		if current, ok := models[group]; ok {
			current.add(b)
		} else {
			created := b
			models[group] = &created
		}
		bucketFor(providers, providerKey).add(b)
		bucketFor(scenarios, scenario).add(b)
		bucketFor(days, day).add(b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := &AnalyticsSnapshot{Summary: &TokenSummary{
		TotalRequests:       total.requests,
		KnownRequests:       total.known,
		ErrorRequests:       total.errors,
		InputTokens:         total.input,
		OutputTokens:        total.output,
		CacheReadTokens:     total.cacheRead,
		CacheCreationTokens: total.cacheCreation,
		SuccessRate:         total.successRate(),
		EstCostUSD:          total.cost,
		UnknownCostRequests: total.requests - total.costRows,
		PeriodStart:         window.trusted,
		PeriodEnd:           window.end,
	}}

	// The orderings reproduce the ORDER BY each query used to carry, so the
	// panels rank models and platforms exactly as they did.
	modelKeys := make([]modelGroup, 0, len(models))
	for key := range models {
		modelKeys = append(modelKeys, key)
	}
	sort.Slice(modelKeys, func(i, j int) bool {
		left, right := models[modelKeys[i]], models[modelKeys[j]]
		if left.requests != right.requests {
			return left.requests > right.requests
		}
		if modelKeys[i].provider != modelKeys[j].provider {
			return modelKeys[i].provider < modelKeys[j].provider
		}
		return modelKeys[i].model < modelKeys[j].model
	})
	out.Models = make([]ModelBreakdown, 0, len(modelKeys))
	for _, key := range modelKeys {
		b := models[key]
		out.Models = append(out.Models, ModelBreakdown{
			Model: key.model, Provider: key.provider,
			Requests: b.requests, KnownRequests: b.known,
			InputTokens: b.input, OutputTokens: b.output,
			CacheReadTokens: b.cacheRead, CacheCreationTokens: b.cacheCreation,
			AvgLatencyMs: b.avgLatencyMs(), SuccessRate: b.successRate(),
			TokensPerSecond: b.tokensPerSecond(),
			EstCostUSD:      b.cost, UnknownCostRequests: b.requests - b.costRows,
		})
	}

	providerKeys := make([]string, 0, len(providers))
	for key := range providers {
		providerKeys = append(providerKeys, key)
	}
	sort.Slice(providerKeys, func(i, j int) bool {
		left, right := providers[providerKeys[i]], providers[providerKeys[j]]
		if left.requests != right.requests {
			return left.requests > right.requests
		}
		return providerKeys[i] < providerKeys[j]
	})
	out.Providers = make([]ProviderBreakdown, 0, len(providerKeys))
	for _, key := range providerKeys {
		b := providers[key]
		fallbackRate := 0.0
		if b.known > 0 {
			fallbackRate = 100.0 * float64(b.fallbacks) / float64(b.known)
		}
		out.Providers = append(out.Providers, ProviderBreakdown{
			Provider: key, Requests: b.requests, KnownRequests: b.known,
			InputTokens: b.input, OutputTokens: b.output,
			CacheReadTokens: b.cacheRead, CacheCreationTokens: b.cacheCreation,
			AvgLatencyMs: b.avgLatencyMs(), SuccessRate: b.successRate(),
			TokensPerSecond: b.tokensPerSecond(), FallbackRate: fallbackRate,
			EstCostUSD: b.cost, UnknownCostRequests: b.requests - b.costRows,
		})
	}

	scenarioKeys := make([]string, 0, len(scenarios))
	for key := range scenarios {
		scenarioKeys = append(scenarioKeys, key)
	}
	sort.Slice(scenarioKeys, func(i, j int) bool {
		left, right := scenarios[scenarioKeys[i]], scenarios[scenarioKeys[j]]
		if left.requests != right.requests {
			return left.requests > right.requests
		}
		return scenarioKeys[i] < scenarioKeys[j]
	})
	out.Scenarios = make([]ScenarioBreakdown, 0, len(scenarioKeys))
	for _, key := range scenarioKeys {
		b := scenarios[key]
		out.Scenarios = append(out.Scenarios, ScenarioBreakdown{
			Scenario: key, Requests: b.requests, KnownRequests: b.known,
			InputTokens: b.input, OutputTokens: b.output,
			CacheReadTokens: b.cacheRead, CacheCreationTokens: b.cacheCreation,
			SuccessRate: b.successRate(),
			EstCostUSD:  b.cost, UnknownCostRequests: b.requests - b.costRows,
		})
	}

	dayKeys := make([]string, 0, len(days))
	for key := range days {
		dayKeys = append(dayKeys, key)
	}
	sort.Strings(dayKeys)
	out.Trend = make([]DailyTokenPoint, 0, len(dayKeys))
	for _, key := range dayKeys {
		b := days[key]
		out.Trend = append(out.Trend, DailyTokenPoint{
			Date: key, Requests: b.requests, KnownRequests: b.known,
			ErrorRequests:   b.errors,
			InputTokens:     b.input,
			OutputTokens:    b.output,
			CacheReadTokens: b.cacheRead, CacheCreationTokens: b.cacheCreation,
			CostUSD: b.cost, UnknownCostRequests: b.requests - b.costRows,
		})
	}
	return out, nil
}

// TokenSummary returns aggregated token/request metrics for a resolved window.
// Corrected rows remain included through usage_trusted semantics.
func (a *Analytics) TokenSummary(window Window) (*TokenSummary, error) {
	snapshot, err := a.Summary(window)
	if err != nil {
		return nil, err
	}
	return snapshot.Summary, nil
}

// ModelBreakdown returns usage stats per model for a resolved window.
func (a *Analytics) ModelBreakdown(window Window) ([]ModelBreakdown, error) {
	snapshot, err := a.Summary(window)
	if err != nil {
		return nil, err
	}
	return snapshot.Models, nil
}

// ScenarioBreakdown returns local usage grouped by routing scenario for a
// resolved window.
func (a *Analytics) ScenarioBreakdown(window Window) ([]ScenarioBreakdown, error) {
	snapshot, err := a.Summary(window)
	if err != nil {
		return nil, err
	}
	return snapshot.Scenarios, nil
}

// ProviderBreakdown returns usage by provider (with fallback rate) for a
// resolved window.
func (a *Analytics) ProviderBreakdown(window Window) ([]ProviderBreakdown, error) {
	snapshot, err := a.Summary(window)
	if err != nil {
		return nil, err
	}
	return snapshot.Providers, nil
}

// TokenTrend returns hourly or daily aggregates for a resolved window.
//
// A daily trend reuses the grouped snapshot. An hourly trend needs its own
// query: no panel asks for it by default, and grouping the same rows into 689
// hour buckets costs about as much as the whole daily snapshot.
func (a *Analytics) TokenTrend(window Window, granularity string) ([]DailyTokenPoint, error) {
	switch granularity {
	case "day":
		snapshot, err := a.Summary(window)
		if err != nil {
			return nil, err
		}
		return snapshot.Trend, nil
	case "hour":
		return a.hourlyTrend(window)
	default:
		return nil, errors.New("analytics granularity must be day or hour")
	}
}

// hourlyTrend buckets the window by UTC hour.
//
// Cached like the snapshot: the analytics tab polls every three seconds while
// it is open, and an hourly view is the one configuration that pays for this
// query on every poll.
func (a *Analytics) hourlyTrend(window Window) ([]DailyTokenPoint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	revision, err := a.db.historyRevision(ctx)
	if err != nil {
		return nil, err
	}
	key := window.cacheKey()
	if cached, ok := a.db.cachedHourly(key, revision); ok {
		return cached, nil
	}

	result, err := a.queryHourly(ctx, window)
	if err != nil {
		return nil, err
	}
	a.db.storeHourly(key, revision, result)
	return result, nil
}

func (a *Analytics) queryHourly(ctx context.Context, window Window) ([]DailyTokenPoint, error) {
	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT strftime('%Y-%m-%dT%H:00:00Z', r.start_time) AS bucket,
		       COUNT(*),
		       COUNT(CASE WHEN r.details_known = 1 AND r.success IN (0, 1) THEN 1 END),
		       COALESCE(SUM(CASE WHEN r.details_known = 1 AND r.success = 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(r.input_tokens), 0),
		       COALESCE(SUM(r.output_tokens), 0),
		       COALESCE(SUM(r.cache_read_tokens), 0),
		       COALESCE(SUM(r.cache_creation_tokens), 0),
		       COALESCE(SUM(r.cost_usd), 0),
		       COUNT(r.cost_usd)
		FROM requests r
		WHERE julianday(r.start_time) >= julianday(?)
		  AND julianday(r.start_time) < julianday(?)
		  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
		  AND (? = '' OR r.provider = ?)
		GROUP BY bucket
		ORDER BY bucket
	`, window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano),
		window.trusted.Format(time.RFC3339Nano), window.provider, window.provider)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]DailyTokenPoint, 0)
	for rows.Next() {
		var point DailyTokenPoint
		var costRows int64
		if err := rows.Scan(&point.Date, &point.Requests, &point.KnownRequests, &point.ErrorRequests,
			&point.InputTokens, &point.OutputTokens, &point.CacheReadTokens,
			&point.CacheCreationTokens, &point.CostUSD, &costRows); err != nil {
			return nil, err
		}
		point.UnknownCostRequests = point.Requests - costRows
		result = append(result, point)
	}
	return result, rows.Err()
}

// analyticsCacheLimit bounds the snapshot cache. The dashboard offers a handful
// of date ranges and platforms; the limit exists so a script sweeping query
// parameters cannot grow the map without bound.
const analyticsCacheLimit = 32

// cachedAnalytics returns the snapshot computed for this window, if the request
// table still holds the same rows.
func (d *Database) cachedAnalytics(key, revision string) (*AnalyticsSnapshot, bool) {
	d.analyticsMu.Lock()
	defer d.analyticsMu.Unlock()
	if d.analyticsRevision != revision {
		return nil, false
	}
	snapshot, ok := d.analytics[key]
	return snapshot, ok
}

func (d *Database) storeAnalytics(key, revision string, snapshot *AnalyticsSnapshot) {
	d.analyticsMu.Lock()
	defer d.analyticsMu.Unlock()
	if d.analyticsRevision != revision || len(d.analytics) >= analyticsCacheLimit {
		d.analytics = make(map[string]*AnalyticsSnapshot, analyticsCacheLimit)
		d.analyticsRevision = revision
	}
	d.analytics[key] = snapshot
}

// cachedHourly and storeHourly carry the hourly trend, which is a separate
// query with its own lifetime. It shares the snapshot's bound and its revision
// validation but not its map: an hourly view must not evict the daily snapshot
// the rest of the tab is reading.
func (d *Database) cachedHourly(key, revision string) ([]DailyTokenPoint, bool) {
	d.analyticsMu.Lock()
	defer d.analyticsMu.Unlock()
	if d.hourlyRevision != revision {
		return nil, false
	}
	points, ok := d.hourly[key]
	return points, ok
}

func (d *Database) storeHourly(key, revision string, points []DailyTokenPoint) {
	d.analyticsMu.Lock()
	defer d.analyticsMu.Unlock()
	if d.hourlyRevision != revision || len(d.hourly) >= analyticsCacheLimit {
		d.hourly = make(map[string][]DailyTokenPoint, analyticsCacheLimit)
		d.hourlyRevision = revision
	}
	d.hourly[key] = points
}
