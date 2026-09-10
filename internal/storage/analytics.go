package storage

import (
	"context"
	"errors"
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

// Window returns the window covering the last N days. Non-positive days means 30.
func (a *Analytics) Window(days int) Window {
	if days <= 0 {
		days = 30
	}
	now := time.Now().UTC()
	// SQLite stores request timestamps at millisecond precision. Leave a small
	// inclusive tolerance so a request inserted immediately before aggregation
	// cannot round to the half-open window's upper boundary.
	return a.window(now.AddDate(0, 0, -days), now.Add(time.Second))
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

// TokenSummary returns aggregated token/request metrics for a resolved window.
// Corrected rows remain included through usage_trusted semantics.
func (a *Analytics) TokenSummary(window Window) (*TokenSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var summary TokenSummary
	summary.PeriodStart = window.trusted
	summary.PeriodEnd = window.end

	row := a.db.DB().QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS total_requests,
			COUNT(CASE WHEN details_known = 1 AND success IN (0, 1) THEN 1 END) AS known_requests,
			COALESCE(SUM(CASE WHEN details_known = 1 AND success = 0 THEN 1 ELSE 0 END), 0) AS error_requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(AVG(CASE WHEN details_known = 1 AND success IN (0, 1) THEN success END), 0) AS success_rate,
			COALESCE(SUM(cost_usd), 0), COUNT(*) - COUNT(cost_usd)
			FROM requests r
			WHERE julianday(r.start_time) >= julianday(?)
			  AND julianday(r.start_time) < julianday(?)
				  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
				  AND (? = '' OR r.provider = ?)
			`, window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano), window.provider, window.provider)

	var scanErr error
	if scanErr = row.Scan(&summary.TotalRequests, &summary.KnownRequests, &summary.ErrorRequests, &summary.InputTokens, &summary.OutputTokens, &summary.CacheReadTokens, &summary.CacheCreationTokens,
		&summary.SuccessRate, &summary.EstCostUSD, &summary.UnknownCostRequests); scanErr != nil {
		return nil, scanErr
	}
	return &summary, nil
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
	EstCostUSD          float64 `json:"est_cost_usd"` // sum of known stored request costs
	UnknownCostRequests int64   `json:"unknown_cost_requests"`
}

// ModelBreakdown returns usage stats per model for a resolved window.
func (a *Analytics) ModelBreakdown(window Window) ([]ModelBreakdown, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT
			r.model,
			COALESCE(r.provider, '') AS provider,
			COUNT(*) AS requests,
			COUNT(CASE WHEN r.details_known = 1 AND r.success IN (0, 1) THEN 1 END) AS known_requests,
			COALESCE(SUM(r.input_tokens), 0) AS input_tokens,
			COALESCE(SUM(r.output_tokens), 0) AS output_tokens,
			COALESCE(SUM(r.cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(r.cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(AVG(CASE WHEN r.details_known = 1 AND r.duration_ms > 0 AND julianday(r.start_time) >= julianday(?) THEN r.duration_ms END), 0) AS avg_latency_ms,
			COALESCE(AVG(CASE WHEN r.details_known = 1 AND r.success IN (0, 1) THEN r.success END), 0) AS success_rate,
			COALESCE(SUM(r.cost_usd), 0) AS stored_cost_usd,
			COUNT(*) - COUNT(r.cost_usd) AS unknown_cost_requests
		FROM requests r
			WHERE julianday(r.start_time) >= julianday(?)
			  AND julianday(r.start_time) < julianday(?)
				  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
				  AND (? = '' OR r.provider = ?)
			GROUP BY r.model, COALESCE(r.provider, '')
		ORDER BY requests DESC, provider ASC, r.model ASC
			`, window.trusted.Format(time.RFC3339Nano),
		window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano), window.provider, window.provider)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []ModelBreakdown
	for rows.Next() {
		var mb ModelBreakdown
		if err := rows.Scan(
			&mb.Model,
			&mb.Provider,
			&mb.Requests,
			&mb.KnownRequests,
			&mb.InputTokens,
			&mb.OutputTokens,
			&mb.CacheReadTokens,
			&mb.CacheCreationTokens,
			&mb.AvgLatencyMs,
			&mb.SuccessRate,
			&mb.EstCostUSD,
			&mb.UnknownCostRequests,
		); err != nil {
			return nil, err
		}
		result = append(result, mb)
	}
	return result, rows.Err()
}

// costForTokens prices OpenCode Go token counts in USD. Rates come from the
// embedded seed rules; the models table (which has no cache columns) is used
// only as a fallback for input/output when the model has no seed rule, so
// catalog-synced or user-overridden prices still surface. Complete request
// estimates must use costForProviderTokensAt to retain unknown-price semantics.
func costForTokens(model string, in, out, cacheRead, cacheCreate int64, modelsInputPerM, modelsOutputPerM float64) float64 {
	ipm, opm, crpm, cwpm, ok := PriceForModel(model)
	if !ok {
		if modelsInputPerM == 0 && modelsOutputPerM == 0 {
			return 0
		}
		// No seed rule: fall back to models-table input/output rates and treat
		// cache creation as input. Cache reads stay unpriced rather than being
		// billed at the full input rate.
		ipm, opm = modelsInputPerM, modelsOutputPerM
	}
	// OpenCode semantics: cache CREATION (first-time prompt write) bills at the
	// cache_write price when the model publishes one, otherwise at the input
	// price; cache READ bills at the cheap cache_read price.
	// `in` is the cache-miss part already (splitPromptTokens strips the cached
	// prefix before storage), so no deduction applies. Verified against the
	// platform invoice: bill = miss*input + hit*cache_read + out*output,
	// never miss-cacheRead (that under-prices rows where miss > hit).
	cacheWriteRate := ipm
	if cwpm > 0 {
		cacheWriteRate = cwpm
	}
	return (float64(in)*ipm +
		float64(cacheCreate)*cacheWriteRate +
		float64(cacheRead)*crpm +
		float64(out)*opm) / 1_000_000
}

// ProviderBreakdown holds per-provider aggregates.
type ProviderBreakdown struct {
	Provider            string  `json:"provider"`
	Requests            int64   `json:"requests"`
	KnownRequests       int64   `json:"known_requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
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

// ScenarioBreakdown returns local usage grouped by routing scenario for a
// resolved window.
func (a *Analytics) ScenarioBreakdown(window Window) ([]ScenarioBreakdown, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT CASE WHEN LOWER(TRIM(COALESCE(r.scenario, ''))) IN ('', 'unknown') THEN 'override' ELSE r.scenario END AS scenario,
		       COUNT(*) AS requests, COUNT(CASE WHEN r.details_known = 1 AND r.success IN (0, 1) THEN 1 END),
		       COALESCE(SUM(r.input_tokens), 0), COALESCE(SUM(r.output_tokens), 0),
		       COALESCE(SUM(r.cache_read_tokens), 0), COALESCE(SUM(r.cache_creation_tokens), 0),
		       COALESCE(AVG(CASE WHEN r.details_known = 1 AND r.success IN (0, 1) THEN r.success END), 0),
		       COALESCE(SUM(r.cost_usd), 0), COUNT(*) - COUNT(r.cost_usd)
		FROM requests r
			WHERE julianday(r.start_time) >= julianday(?)
			  AND julianday(r.start_time) < julianday(?)
				  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
				  AND (? = '' OR r.provider = ?)
				GROUP BY CASE WHEN LOWER(TRIM(COALESCE(r.scenario, ''))) IN ('', 'unknown') THEN 'override' ELSE r.scenario END
			ORDER BY requests DESC, scenario ASC
			`, window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano), window.provider, window.provider)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]ScenarioBreakdown, 0)
	for rows.Next() {
		var row ScenarioBreakdown
		if err := rows.Scan(&row.Scenario, &row.Requests, &row.KnownRequests, &row.InputTokens, &row.OutputTokens,
			&row.CacheReadTokens, &row.CacheCreationTokens, &row.SuccessRate, &row.EstCostUSD, &row.UnknownCostRequests); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ProviderBreakdown returns usage by provider (with fallback rate) for a
// resolved window.
func (a *Analytics) ProviderBreakdown(window Window) ([]ProviderBreakdown, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT
			COALESCE(r.provider, 'unknown') AS provider,
			COUNT(*) AS requests,
			COUNT(CASE WHEN r.details_known = 1 AND r.success IN (0, 1) THEN 1 END) AS known_requests,
			COALESCE(SUM(r.input_tokens), 0) AS input_tokens,
			COALESCE(SUM(r.output_tokens), 0) AS output_tokens,
			COALESCE(SUM(r.cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(r.cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(100.0 * AVG(CASE WHEN r.details_known = 1 AND r.success IN (0, 1)
			                        THEN COALESCE(r.attempt, 1) > 1 END), 0) AS fallback_rate,
			COALESCE(SUM(r.cost_usd), 0) AS stored_cost_usd,
			COUNT(*) - COUNT(r.cost_usd) AS unknown_cost_requests
		FROM requests r
			WHERE julianday(r.start_time) >= julianday(?)
			  AND julianday(r.start_time) < julianday(?)
				  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
				  AND (? = '' OR r.provider = ?)
				GROUP BY r.provider
			ORDER BY requests DESC, provider ASC
		`, window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano), window.provider, window.provider)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]ProviderBreakdown, 0)
	for rows.Next() {
		var row ProviderBreakdown
		if err := rows.Scan(&row.Provider, &row.Requests, &row.KnownRequests, &row.InputTokens, &row.OutputTokens,
			&row.CacheReadTokens, &row.CacheCreationTokens, &row.FallbackRate, &row.EstCostUSD, &row.UnknownCostRequests); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
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

// TokenTrend returns hourly or daily aggregates for a resolved window.
func (a *Analytics) TokenTrend(window Window, granularity string) ([]DailyTokenPoint, error) {
	bucket := "DATE(r.start_time)"
	if granularity == "hour" {
		bucket = "strftime('%Y-%m-%dT%H:00:00Z', r.start_time)"
	} else if granularity != "day" {
		return nil, errors.New("analytics granularity must be day or hour")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT
			`+bucket+` AS bucket,
			COUNT(*) AS requests,
			COUNT(CASE WHEN details_known = 1 AND success IN (0, 1) THEN 1 END) AS known_requests,
			COALESCE(SUM(CASE WHEN details_known = 1 AND success = 0 THEN 1 ELSE 0 END), 0) AS error_requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(SUM(cost_usd), 0) AS cost_usd,
			COUNT(*) - COUNT(cost_usd) AS unknown_cost_requests
		FROM requests r
		WHERE julianday(r.start_time) >= julianday(?)
		  AND julianday(r.start_time) < julianday(?)
			  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
			  AND (? = '' OR r.provider = ?)
			GROUP BY `+bucket+`
		ORDER BY bucket ASC
	`, window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano), window.provider, window.provider)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []DailyTokenPoint
	for rows.Next() {
		var p DailyTokenPoint
		if err := rows.Scan(&p.Date, &p.Requests, &p.KnownRequests, &p.ErrorRequests, &p.InputTokens, &p.OutputTokens, &p.CacheReadTokens, &p.CacheCreationTokens, &p.CostUSD, &p.UnknownCostRequests); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
