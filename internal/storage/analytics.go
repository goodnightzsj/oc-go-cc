package storage

import (
	"context"
	"sort"
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

// SetBaseline makes every aggregate ignore requests recorded before t. Pass the
// zero time to analyse the full history again.
func (a *Analytics) SetBaseline(t time.Time) {
	a.baseline = t
}

// Baseline reports the current cutoff; the zero time means no cutoff.
func (a *Analytics) Baseline() time.Time {
	return a.baseline
}

// windowStart clamps a requested "last N days" start to the baseline, so all
// aggregates share one definition of where trustworthy data begins.
func (a *Analytics) windowStart(days int) time.Time {
	since := time.Now().AddDate(0, 0, -days)
	if !a.baseline.IsZero() && a.baseline.After(since) {
		return a.baseline
	}
	return since
}

// TokenSummary holds high-level token and request metrics for a time window.
type TokenSummary struct {
	TotalRequests       int64     `json:"total_requests"`
	InputTokens         int64     `json:"input_tokens"`
	OutputTokens        int64     `json:"output_tokens"`
	CacheReadTokens     int64     `json:"cache_read_tokens"`
	CacheCreationTokens int64     `json:"cache_creation_tokens"`
	SuccessRate         float64   `json:"success_rate"` // 0-1
	EstCostUSD          float64   `json:"est_cost_usd"`
	ProviderCostRows    int64     `json:"provider_cost_rows"`
	EstimatedCostRows   int64     `json:"estimated_cost_rows"`
	PeriodStart         time.Time `json:"period_start"`
	PeriodEnd           time.Time `json:"period_end"`
}

// GetTokenSummary returns aggregated token/request metrics for the last N days.
func (a *Analytics) GetTokenSummary(days int) (*TokenSummary, error) {
	if days <= 0 {
		days = 30
	}
	since := a.windowStart(days)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var summary TokenSummary
	summary.PeriodStart = since
	summary.PeriodEnd = time.Now()

	row := a.db.DB().QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS total_requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
			CASE
				WHEN COUNT(*) > 0 THEN CAST(SUM(success) AS FLOAT) / COUNT(*)
				ELSE 0
			END AS success_rate,
			COUNT(CASE WHEN cost_source = 'provider' THEN 1 END) AS provider_cost_rows,
			COUNT(CASE WHEN cost_usd IS NOT NULL AND COALESCE(cost_source, 'estimated') != 'provider' THEN 1 END) AS estimated_cost_rows
		FROM requests r
		WHERE r.start_time >= ?
	`, since.Format(time.RFC3339Nano))

	var scanErr error
	if scanErr = row.Scan(&summary.TotalRequests, &summary.InputTokens, &summary.OutputTokens, &summary.CacheReadTokens, &summary.CacheCreationTokens,
		&summary.SuccessRate, &summary.ProviderCostRows, &summary.EstimatedCostRows); scanErr != nil {
		return nil, scanErr
	}

	// Compute estimated cost in Go using per-model price rules. This stays
	// correct even when the catalog/models table is empty (fresh installs):
	// each model's input/output is priced from the embedded seed rules and
	// summed, instead of a single models-table JOIN that yields NULL/0.
	cost, costErr := a.modelCostSum(ctx, since.Format(time.RFC3339Nano))
	if costErr != nil {
		return nil, costErr
	}
	summary.EstCostUSD = cost
	return &summary, nil
}

// modelCostSum sums estimated USD cost across requests using price rules from
// seed_prices.json, keyed per model. Falls back to the models table for
// input/output rates when a model has no seed rule.
func (a *Analytics) modelCostSum(ctx context.Context, since string) (float64, error) {
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
		WHERE r.start_time >= ?
		GROUP BY r.model
	`, since)
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

// TotalTokens is the full input/output/cache volume for ring-chart weighting.
func (mb ModelBreakdown) TotalTokens() int64 {
	return mb.InputTokens + mb.OutputTokens + mb.CacheReadTokens + mb.CacheCreationTokens
}

// GetModelBreakdown returns usage stats per model for the last N days.
func (a *Analytics) GetModelBreakdown(days int) ([]ModelBreakdown, error) {
	if days <= 0 {
		days = 30
	}
	since := a.windowStart(days)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT
			r.model,
			COALESCE(r.provider, '') AS provider,
			COUNT(*) AS requests,
			COALESCE(SUM(r.input_tokens), 0) AS input_tokens,
			COALESCE(SUM(r.output_tokens), 0) AS output_tokens,
			COALESCE(SUM(r.cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(r.cache_creation_tokens), 0) AS cache_creation_tokens,
			COALESCE(l.avg_latency, 0) AS avg_latency_ms,
			CASE
				WHEN COUNT(*) > 0 THEN CAST(SUM(r.success) AS FLOAT) / COUNT(*)
				ELSE 0
			END AS success_rate,
			COALESCE(m.cost_input_per_m, 0) AS models_cost_input_per_m,
			COALESCE(m.cost_output_per_m, 0) AS models_cost_output_per_m,
			COUNT(r.cost_usd) AS cost_rows,
			COALESCE(SUM(r.cost_usd), 0) AS stored_cost_usd
		FROM requests r
		LEFT JOIN (
			SELECT model, AVG(latency_ms) AS avg_latency
			FROM latency_samples
			WHERE recorded_at >= ?
			GROUP BY model
		) l ON l.model = r.model
		LEFT JOIN models m
			ON m.id = r.model
		WHERE r.start_time >= ?
		GROUP BY r.model, r.provider
		ORDER BY requests DESC
	`, since.Format(time.RFC3339Nano), since.Format(time.RFC3339Nano))
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
	Provider     string  `json:"provider"`
	Requests     int64   `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	FallbackRate float64 `json:"fallback_rate"` // % of requests that were fallbacks
	EstCostUSD   float64 `json:"est_cost_usd"`
}

// GetProviderBreakdown returns usage by provider (with fallback rate).
func (a *Analytics) GetProviderBreakdown(days int) ([]ProviderBreakdown, error) {
	if days <= 0 {
		days = 30
	}
	since := a.windowStart(days)

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
		WHERE r.start_time >= ?
		GROUP BY r.provider, r.model
	`, since.Format(time.RFC3339Nano))
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
	sort.Slice(result, func(i, j int) bool {
		if result[i].Requests != result[j].Requests {
			return result[i].Requests > result[j].Requests
		}
		return result[i].Provider < result[j].Provider
	})
	return result, nil
}

// DailyTokenPoint is a single day in the token trend.
type DailyTokenPoint struct {
	Date                string `json:"date"` // YYYY-MM-DD
	Requests            int64  `json:"requests"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
}

// GetDailyTokenTrend returns daily token/request aggregates for the last N days.
func (a *Analytics) GetDailyTokenTrend(days int) ([]DailyTokenPoint, error) {
	if days <= 0 {
		days = 30
	}
	since := a.windowStart(days)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := a.db.DB().QueryContext(ctx, `
		SELECT
			DATE(start_time) AS day,
			COUNT(*) AS requests,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens
		FROM requests
		WHERE start_time >= ?
		GROUP BY DATE(start_time)
		ORDER BY day ASC
	`, since.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []DailyTokenPoint
	for rows.Next() {
		var p DailyTokenPoint
		if err := rows.Scan(&p.Date, &p.Requests, &p.InputTokens, &p.OutputTokens, &p.CacheReadTokens, &p.CacheCreationTokens); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
