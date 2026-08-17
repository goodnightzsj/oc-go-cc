package storage

import (
	"cmp"
	"context"
	"errors"
	"slices"
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
}

// Window returns the window covering the last N days. Non-positive days means 30.
func (a *Analytics) Window(days int) Window {
	if days <= 0 {
		days = 30
	}
	now := time.Now()
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
	return Window{requested: start, trusted: trusted, end: end}
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
			COALESCE(SUM(details_known), 0) AS known_requests,
			COALESCE(SUM(CASE WHEN details_known = 1 AND success = 0 THEN 1 ELSE 0 END), 0) AS error_requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
			CASE
				WHEN SUM(details_known) > 0 THEN CAST(SUM(CASE WHEN details_known = 1 THEN success ELSE 0 END) AS FLOAT) / SUM(details_known)
				ELSE 0
			END AS success_rate
			FROM requests r
			WHERE julianday(r.start_time) >= julianday(?)
			  AND julianday(r.start_time) < julianday(?)
			  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
		`, window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano))

	var scanErr error
	if scanErr = row.Scan(&summary.TotalRequests, &summary.KnownRequests, &summary.ErrorRequests, &summary.InputTokens, &summary.OutputTokens, &summary.CacheReadTokens, &summary.CacheCreationTokens,
		&summary.SuccessRate); scanErr != nil {
		return nil, scanErr
	}

	// Compute estimated cost in Go using per-model price rules. This stays
	// correct even when the catalog/models table is empty (fresh installs):
	// each model's input/output is priced from the embedded seed rules and
	// summed, instead of a single models-table JOIN that yields NULL/0.
	cost, costErr := a.modelCostSum(ctx, window)
	if costErr != nil {
		return nil, costErr
	}
	summary.EstCostUSD = cost
	return &summary, nil
}

// modelCostSum sums estimated USD cost across requests using price rules from
// seed_prices.json, keyed per model. Falls back to the models table for
// input/output rates when a model has no seed rule.
func (a *Analytics) modelCostSum(ctx context.Context, window Window) (float64, error) {
	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT r.model,
		       COUNT(*),
		       COUNT(r.cost_usd),
		       COALESCE(SUM(r.cost_usd), 0),
		       COALESCE(SUM(r.input_tokens), 0),
		       COALESCE(SUM(r.output_tokens), 0),
		       COALESCE(SUM(r.cache_read_tokens), 0),
		       COALESCE(SUM(r.cache_creation_tokens), 0),
		       COALESCE(m.cost_input_per_m, 0),
		       COALESCE(m.cost_output_per_m, 0)
			FROM requests r
			LEFT JOIN models m ON m.id = r.model
			WHERE julianday(r.start_time) >= julianday(?)
			  AND julianday(r.start_time) < julianday(?)
			  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
			GROUP BY r.model
		`, window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	var cost float64
	for rows.Next() {
		var model string
		var requests, costRows int64
		var storedCost float64
		var inToks, outToks, cacheReadToks, cacheCreateToks int64
		var modelsInputPerM, modelsOutputPerM float64
		if err := rows.Scan(&model, &requests, &costRows, &storedCost, &inToks, &outToks, &cacheReadToks, &cacheCreateToks,
			&modelsInputPerM, &modelsOutputPerM); err != nil {
			return 0, err
		}
		if costRows == requests {
			cost += storedCost
			continue
		}
		cost += costForTokens(model, inToks, outToks, cacheReadToks, cacheCreateToks,
			modelsInputPerM, modelsOutputPerM)
	}
	return cost, rows.Err()
}

// ModelBreakdown holds per-model usage and performance stats.
type ModelBreakdown struct {
	Model               string  `json:"model"`
	Provider            string  `json:"provider"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	AvgLatencyMs        float64 `json:"avg_latency_ms"`
	SuccessRate         float64 `json:"success_rate"`
	EstCostUSD          float64 `json:"est_cost_usd"` // based on models.cost_* if available
}

// ModelBreakdown returns usage stats per model for a resolved window.
func (a *Analytics) ModelBreakdown(window Window) ([]ModelBreakdown, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT
			r.model,
			COALESCE(MAX(r.provider), '') AS provider,
			COUNT(*) AS requests,
			COALESCE(SUM(r.input_tokens), 0) AS input_tokens,
			COALESCE(SUM(r.output_tokens), 0) AS output_tokens,
			COALESCE(SUM(r.cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(r.cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(l.avg_latency, 0) AS avg_latency_ms,
			CASE
				WHEN SUM(r.details_known) > 0 THEN CAST(SUM(CASE WHEN r.details_known = 1 THEN r.success ELSE 0 END) AS FLOAT) / SUM(r.details_known)
				ELSE 0
			END AS success_rate,
			COALESCE(m.cost_input_per_m, 0) AS models_cost_input_per_m,
			COALESCE(m.cost_output_per_m, 0) AS models_cost_output_per_m,
			COUNT(r.cost_usd) AS cost_rows,
			COALESCE(SUM(r.cost_usd), 0) AS stored_cost_usd
		FROM requests r
			LEFT JOIN (
				SELECT model, AVG(duration_ms) AS avg_latency
				FROM requests
				WHERE julianday(start_time) >= julianday(?)
				  AND julianday(start_time) < julianday(?)
				  AND duration_ms > 0
				  AND details_known = 1
				GROUP BY model
			) l ON l.model = r.model
		LEFT JOIN models m
			ON m.id = r.model
			WHERE julianday(r.start_time) >= julianday(?)
			  AND julianday(r.start_time) < julianday(?)
			  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
		GROUP BY r.model
		ORDER BY requests DESC
		`, window.trusted.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano),
		window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []ModelBreakdown
	for rows.Next() {
		var mb ModelBreakdown
		var modelsInputPerM, modelsOutputPerM float64
		var costRows int64
		var storedCost float64
		if err := rows.Scan(
			&mb.Model,
			&mb.Provider,
			&mb.Requests,
			&mb.InputTokens,
			&mb.OutputTokens,
			&mb.CacheReadTokens,
			&mb.CacheCreationTokens,
			&mb.AvgLatencyMs,
			&mb.SuccessRate,
			&modelsInputPerM,
			&modelsOutputPerM,
			&costRows,
			&storedCost,
		); err != nil {
			return nil, err
		}
		// Price the row in Go using the same rules as modelCostSum, so the
		// per-model breakdown always adds up to the summary total. The old SQL
		// expression priced only input+output at the models-table rates, which
		// silently billed every cached token at the full input rate and left
		// the breakdown disagreeing with the headline cost.
		if costRows == mb.Requests {
			mb.EstCostUSD = storedCost
		} else {
			mb.EstCostUSD = costForTokens(mb.Model,
				mb.InputTokens, mb.OutputTokens, mb.CacheReadTokens, mb.CacheCreationTokens,
				modelsInputPerM, modelsOutputPerM)
		}
		result = append(result, mb)
	}
	return result, rows.Err()
}

// costForTokens prices one model's token counts in USD. Rates come from the
// embedded seed rules; the models table (which has no cache columns) is used
// only as a fallback for input/output when the model has no seed rule, so
// catalog-synced or user-overridden prices still surface.
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
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	FallbackRate        float64 `json:"fallback_rate"` // % of requests that were fallbacks
	EstCostUSD          float64 `json:"est_cost_usd"`
}

// ScenarioBreakdown holds proxy-owned request aggregates. Scenarios are not
// available in the OpenCode account export, so this remains a local drill-down.
type ScenarioBreakdown struct {
	Scenario            string  `json:"scenario"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	SuccessRate         float64 `json:"success_rate"`
	EstCostUSD          float64 `json:"est_cost_usd"`
}

// ScenarioBreakdown returns local usage grouped by routing scenario for a
// resolved window.
func (a *Analytics) ScenarioBreakdown(window Window) ([]ScenarioBreakdown, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT CASE WHEN LOWER(TRIM(COALESCE(r.scenario, ''))) IN ('', 'unknown') THEN 'override' ELSE r.scenario END, r.model,
		       COUNT(*), COALESCE(SUM(r.input_tokens), 0), COALESCE(SUM(r.output_tokens), 0),
		       COALESCE(SUM(r.cache_read_tokens), 0), COALESCE(SUM(r.cache_creation_tokens), 0),
		       COALESCE(SUM(r.success), 0), COUNT(r.cost_usd), COALESCE(SUM(r.cost_usd), 0),
		       COALESCE(m.cost_input_per_m, 0), COALESCE(m.cost_output_per_m, 0)
		FROM requests r
			LEFT JOIN models m ON m.id = r.model
			WHERE julianday(r.start_time) >= julianday(?)
			  AND julianday(r.start_time) < julianday(?)
			  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
			GROUP BY CASE WHEN LOWER(TRIM(COALESCE(r.scenario, ''))) IN ('', 'unknown') THEN 'override' ELSE r.scenario END, r.model
		`, window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type scenarioAgg struct {
		row       ScenarioBreakdown
		successes int64
	}
	byScenario := make(map[string]*scenarioAgg)
	for rows.Next() {
		var scenario, model string
		var requests, in, out, cacheRead, cacheCreate, successes, costRows int64
		var storedCost, inputRate, outputRate float64
		if err := rows.Scan(&scenario, &model, &requests, &in, &out, &cacheRead, &cacheCreate,
			&successes, &costRows, &storedCost, &inputRate, &outputRate); err != nil {
			return nil, err
		}
		agg := byScenario[scenario]
		if agg == nil {
			agg = &scenarioAgg{row: ScenarioBreakdown{Scenario: scenario}}
			byScenario[scenario] = agg
		}
		agg.row.Requests += requests
		agg.row.InputTokens += in
		agg.row.OutputTokens += out
		agg.row.CacheReadTokens += cacheRead
		agg.row.CacheCreationTokens += cacheCreate
		agg.successes += successes
		if costRows == requests {
			agg.row.EstCostUSD += storedCost
		} else {
			agg.row.EstCostUSD += costForTokens(model, in, out, cacheRead, cacheCreate, inputRate, outputRate)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]ScenarioBreakdown, 0, len(byScenario))
	for _, agg := range byScenario {
		if agg.row.Requests > 0 {
			agg.row.SuccessRate = float64(agg.successes) / float64(agg.row.Requests)
		}
		result = append(result, agg.row)
	}
	slices.SortFunc(result, func(a, b ScenarioBreakdown) int {
		return cmp.Or(cmp.Compare(b.Requests, a.Requests), cmp.Compare(a.Scenario, b.Scenario))
	})
	return result, nil
}

// ProviderBreakdown returns usage by provider (with fallback rate) for a
// resolved window.
func (a *Analytics) ProviderBreakdown(window Window) ([]ProviderBreakdown, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Grouped by provider *and* model: prices are per-model, so cost has to be
	// accumulated at model granularity and only then summed up per provider.
	// This keeps the provider totals equal to the model-breakdown totals.
	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT
			COALESCE(r.provider, 'unknown') AS provider,
			r.model,
			COUNT(*) AS requests,
			COALESCE(SUM(r.input_tokens), 0) AS input_tokens,
			COALESCE(SUM(r.output_tokens), 0) AS output_tokens,
			COALESCE(SUM(r.cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(r.cache_creation_tokens), 0) AS cache_creation_tokens,
			-- Fallback rate: attempts > 1 are fallbacks; old rows (NULL/0) treated as primary (rate 0)
			COUNT(CASE WHEN COALESCE(r.attempt, 1) > 1 THEN 1 END) AS fallback_count,
			COALESCE(m.cost_input_per_m, 0) AS models_cost_input_per_m,
			COALESCE(m.cost_output_per_m, 0) AS models_cost_output_per_m,
			COUNT(r.cost_usd) AS cost_rows,
			COALESCE(SUM(r.cost_usd), 0) AS stored_cost_usd
		FROM requests r
			LEFT JOIN models m ON m.id = r.model
			WHERE julianday(r.start_time) >= julianday(?)
			  AND julianday(r.start_time) < julianday(?)
			  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
			GROUP BY r.provider, r.model
		`, window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type providerAgg struct {
		pb        ProviderBreakdown
		fallbacks int64
	}
	byProvider := make(map[string]*providerAgg)
	order := make([]string, 0, 8)

	for rows.Next() {
		var (
			provider, model                         string
			requests, in, out, cacheRead, cacheNew  int64
			fallbacks, costRows                     int64
			modelsInRate, modelsOutRate, storedCost float64
		)
		if err := rows.Scan(&provider, &model, &requests, &in, &out, &cacheRead, &cacheNew,
			&fallbacks, &modelsInRate, &modelsOutRate, &costRows, &storedCost); err != nil {
			return nil, err
		}

		agg, ok := byProvider[provider]
		if !ok {
			agg = &providerAgg{pb: ProviderBreakdown{Provider: provider}}
			byProvider[provider] = agg
			order = append(order, provider)
		}
		agg.pb.Requests += requests
		agg.pb.InputTokens += in
		agg.pb.OutputTokens += out
		agg.pb.CacheReadTokens += cacheRead
		agg.pb.CacheCreationTokens += cacheNew
		if costRows == requests {
			agg.pb.EstCostUSD += storedCost
		} else {
			agg.pb.EstCostUSD += costForTokens(model, in, out, cacheRead, cacheNew, modelsInRate, modelsOutRate)
		}
		agg.fallbacks += fallbacks
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]ProviderBreakdown, 0, len(order))
	for _, provider := range order {
		agg := byProvider[provider]
		if agg.pb.Requests > 0 {
			agg.pb.FallbackRate = 100.0 * float64(agg.fallbacks) / float64(agg.pb.Requests)
		}
		result = append(result, agg.pb)
	}
	slices.SortFunc(result, func(a, b ProviderBreakdown) int {
		return cmp.Or(cmp.Compare(b.Requests, a.Requests), cmp.Compare(a.Provider, b.Provider))
	})
	return result, nil
}

// DailyTokenPoint is a single day in the token trend.
type DailyTokenPoint struct {
	Date                string  `json:"date"` // YYYY-MM-DD
	Requests            int64   `json:"requests"`
	KnownRequests       int64   `json:"known_requests"`
	ErrorRequests       int64   `json:"error_requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CostUSD             float64 `json:"cost_usd"`
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
			COALESCE(SUM(details_known), 0) AS known_requests,
			COALESCE(SUM(CASE WHEN details_known = 1 AND success = 0 THEN 1 ELSE 0 END), 0) AS error_requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(SUM(cost_usd), 0) AS cost_usd
		FROM requests r
		WHERE julianday(r.start_time) >= julianday(?)
		  AND julianday(r.start_time) < julianday(?)
		  AND (julianday(r.start_time) >= julianday(?) OR r.usage_trusted = 1)
		GROUP BY `+bucket+`
		ORDER BY bucket ASC
	`, window.requested.Format(time.RFC3339Nano), window.end.Format(time.RFC3339Nano), window.trusted.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []DailyTokenPoint
	for rows.Next() {
		var p DailyTokenPoint
		if err := rows.Scan(&p.Date, &p.Requests, &p.KnownRequests, &p.ErrorRequests, &p.InputTokens, &p.OutputTokens, &p.CacheReadTokens, &p.CacheCreationTokens, &p.CostUSD); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
