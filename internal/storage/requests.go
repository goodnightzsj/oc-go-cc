package storage

import (
	"context"
	"database/sql"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/routatic/proxy/internal/history"
)

const (
	CostSourceEstimated = "estimated"
	CostSourceProvider  = "provider"
)

// Requests provides methods for reading and writing request records.
type Requests struct {
	db *Database
}

// NewRequests creates a new Requests repository backed by the given database.
func NewRequests(db *Database) *Requests {
	return &Requests{db: db}
}

// Insert stores a request record, replacing any existing record with the same ID.
func (r *Requests) Insert(rec history.RequestRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	attempt := rec.Attempt
	if attempt < 1 {
		attempt = 1
	}
	costUSD, costSource, err := r.costForRecord(ctx, rec)
	if err != nil {
		return err
	}

	_, err = r.db.DB().ExecContext(ctx, `
		INSERT OR REPLACE INTO requests (
			id, model, requested_model, provider, scenario, start_time, duration_ms,
			input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, reasoning_tokens,
			cost_usd, cost_source, details_known, usage_trusted, streaming, success, error_msg, attempt,
			peak_multiplier, upstream_channel, upstream_model, first_byte_ms, reasoning_effort,
			key_hint
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rec.ID,
		rec.Model,
		requestedModelForRecord(rec),
		rec.Provider,
		rec.Scenario,
		rec.StartTime.UTC().Format(time.RFC3339Nano),
		rec.Duration.Milliseconds(),
		rec.InputTokens,
		rec.OutputTokens,
		rec.CacheReadTokens,
		rec.CacheCreationTokens,
		rec.ReasoningTokens,
		costUSD,
		costSource,
		boolToInt(!rec.UsageUnobserved),
		boolToInt(rec.Streaming),
		boolToInt(rec.Success),
		rec.ErrorMsg,
		attempt,
		peakMultiplierForRecord(rec),
		nullIfEmpty(rec.UpstreamChannel),
		nullIfEmpty(rec.UpstreamModel),
		nullIfZero(int64(rec.FirstByte.Milliseconds())),
		nullIfEmpty(rec.ReasoningEffort),
		nullIfEmpty(rec.KeyHint),
	)

	return err
}

// requestedModelForRecord returns the model to store, or NULL when the client
// asked for the model that served it.
//
// Storing NULL rather than a copy of Model keeps the column meaningful as a
// filter: "where the client's model differs from the served one" is then a plain
// non-null test, and a row cannot claim a reroute that did not happen. It also
// keeps the column sparse, so an index over it (if one is ever added) stays
// small.
func requestedModelForRecord(rec history.RequestRecord) any {
	if !history.RequestedModelDiffers(rec.RequestedModel, rec.Model) {
		return nil
	}
	return strings.TrimSpace(rec.RequestedModel)
}

// nullIfEmpty keeps "we did not observe a channel" distinct from "we observed
// an empty one". Only the former is a real state, so it is the only one stored.
func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

// nullIfZero keeps "not measured" distinct from "measured as zero". A latency
// of 0ms is not a real observation, so it is the absence that gets stored.
func nullIfZero(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

// RequestQuery describes a filtered, sorted history page. Empty fields keep
// the existing all-records behavior.
type RequestQuery struct {
	Page, PageSize     int
	Search, Model      string
	Provider, Scenario string
	// Channel filters on the upstream channel the gateway reported. Only
	// ClinePass publishes one, so this narrows that platform's rows; a row from
	// any other platform stores NULL and can never match a channel name.
	Channel            string
	CostSource         string
	Start, End         *time.Time
	Success, Streaming *bool
	SortBy, SortOrder  string
}

type RequestSummary struct {
	TotalRequests       int64              `json:"total_requests"`
	SuccessRate         float64            `json:"success_rate"`
	SuccessRows         int64              `json:"success_rows"`
	TotalTokens         int64              `json:"total_tokens"`
	InputTokens         int64              `json:"input_tokens"`
	OutputTokens        int64              `json:"output_tokens"`
	CacheReadTokens     int64              `json:"cache_read_tokens"`
	CacheCreationTokens int64              `json:"cache_creation_tokens"`
	CostUSD             float64            `json:"cost_usd"`
	CostRows            int64              `json:"cost_rows"`
	UnknownCostRequests int64              `json:"unknown_cost_requests"`
	Models              []RequestBreakdown `json:"models"`
	Providers           []RequestBreakdown `json:"providers"`
	Scenarios           []RequestBreakdown `json:"scenarios"`
	Trend               []RequestTrend     `json:"trend"`
}

type RequestBreakdown struct {
	Name                string  `json:"name"`
	Provider            string  `json:"provider,omitempty"` // model breakdowns only
	Requests            int64   `json:"requests"`
	Tokens              int64   `json:"tokens"`
	CostUSD             float64 `json:"cost_usd"`
	UnknownCostRequests int64   `json:"unknown_cost_requests"`
}

type RequestTrend struct {
	Date                string  `json:"date"` // UTC calendar date
	Requests            int64   `json:"requests"`
	Tokens              int64   `json:"tokens"`
	CostUSD             float64 `json:"cost_usd"`
	UnknownCostRequests int64   `json:"unknown_cost_requests"`
}

// scenarioDimension collapses the two spellings of "no scenario was chosen"
// into one label. A row written by the override path and a row whose scenario
// was never recorded mean the same thing to the reader, so they share a bucket
// rather than appearing as two rows that each look like half the traffic.
const scenarioDimension = `CASE WHEN LOWER(TRIM(COALESCE(scenario, ''))) IN ('', 'unknown') THEN 'override' ELSE scenario END`

// requestBucket accumulates one group's aggregates. It is a plain struct rather
// than the wire types so the same fold can feed all four dimensions.
type requestBucket struct {
	requests      int64
	successes     int64
	successRows   int64
	input         int64
	output        int64
	cacheRead     int64
	cacheCreation int64
	tokens        int64
	cost          float64
	costRows      int64
}

func (b *requestBucket) add(o requestBucket) {
	b.requests += o.requests
	b.successes += o.successes
	b.successRows += o.successRows
	b.input += o.input
	b.output += o.output
	b.cacheRead += o.cacheRead
	b.cacheCreation += o.cacheCreation
	b.tokens += o.tokens
	b.cost += o.cost
	b.costRows += o.costRows
}

// modelGroup keys a model breakdown, which is per model *and* platform: the
// same model name served by two platforms is two rows, as it was when the
// database did the grouping.
type modelGroup struct {
	model    string
	provider string
}

// summaryCacheKey identifies a cached summary by the filter that produced it.
// Every field requestWhere reads must appear here, or two different filters
// would share one entry.
func summaryCacheKey(q RequestQuery) string {
	return strings.Join([]string{
		strings.TrimSpace(q.Search), strings.TrimSpace(q.Model), strings.TrimSpace(q.Provider),
		strings.TrimSpace(q.Scenario), strings.TrimSpace(q.Channel), q.CostSource,
		timeArg(q.Start), timeArg(q.End), boolArg(q.Success), boolArg(q.Streaming),
	}, "\x1f")
}

func timeArg(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func boolArg(b *bool) string {
	switch {
	case b == nil:
		return ""
	case *b:
		return "1"
	default:
		return "0"
	}
}

// historyRevision reads the transactional change counter in constant time.
// Unlike a row-count fingerprint it covers UPDATE, replacement and rowid reuse,
// including writes committed by a separate costs-reconcile process.
//
// It hangs off Database because every cached aggregate is validated by it, not
// only the history summary.
func (d *Database) historyRevision(ctx context.Context) (string, error) {
	var revision int64
	if err := d.DB().QueryRowContext(ctx, "SELECT revision FROM history_revision WHERE id = 1").Scan(&revision); err != nil {
		return "", err
	}
	return strconv.FormatInt(revision, 10), nil
}

func (r *Requests) historyRevision(ctx context.Context) (string, error) {
	return r.db.historyRevision(ctx)
}

// Summary aggregates the exact same population as Query, without pagination.
// It uses one scan and caches until any committed request-table change.
//
// This replaced five separate GROUP BY queries - one per dimension plus a total
// - each of which scanned the whole table. The scan, not the aggregation, is
// what costs: modernc.org/sqlite is a pure-Go SQLite, and on a 60k-row history
// a full scan measures around 100ms through this driver against roughly 6ms for
// the same scan through the C CLI. Five of them dominated the history tab's
// load, because the dashboard fetches this endpoint in parallel with the page
// query and waits for the slower of the two.
//
// Grouping by all four dimensions at once multiplies the number of groups - a
// row contributes to a model bucket, a platform bucket, a scenario bucket and a
// day bucket - but the scan happens once. Measured on a production history at
// roughly 2.3x faster end to end, and verified field for field against the five
// queries it replaces: every aggregate of every group matched, so the wire
// response is unchanged.
func (r *Requests) Summary(ctx context.Context, q RequestQuery) (*RequestSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := r.db.lockAggregate(ctx); err != nil {
		return nil, err
	}
	defer func() { <-r.db.aggregateLoad }()

	revision, err := r.historyRevision(ctx)
	if err != nil {
		return nil, err
	}
	key := summaryCacheKey(q)
	if cached, ok := r.db.cachedSummary(key, revision); ok {
		return cached, nil
	}

	summary, err := r.summarize(ctx, q)
	if err != nil {
		return nil, err
	}
	r.db.storeSummary(key, revision, summary)
	return summary, nil
}

func (r *Requests) summarize(ctx context.Context, q RequestQuery) (*RequestSummary, error) {
	where, args := requestWhere(q)

	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT model, COALESCE(provider, ''), `+scenarioDimension+`, DATE(start_time),
		       COUNT(*),
		       COALESCE(SUM(CASE WHEN details_known = 1 AND success = 1 THEN 1 ELSE 0 END), 0),
		       COUNT(CASE WHEN details_known = 1 AND success IN (0, 1) THEN 1 END),
		       COALESCE(SUM(COALESCE(input_tokens, 0)), 0),
		       COALESCE(SUM(COALESCE(output_tokens, 0)), 0),
		       COALESCE(SUM(COALESCE(cache_read_tokens, 0)), 0),
		       COALESCE(SUM(COALESCE(cache_creation_tokens, 0)), 0),
		       COALESCE(SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0) +
		                    COALESCE(cache_read_tokens, 0) + COALESCE(cache_creation_tokens, 0)), 0),
		       COALESCE(SUM(cost_usd), 0), COUNT(cost_usd)
		FROM requests`+where+`
		GROUP BY 1, 2, 3, 4`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	models := map[modelGroup]*requestBucket{}
	providers := map[string]*requestBucket{}
	scenarios := map[string]*requestBucket{}
	days := map[string]*requestBucket{}
	var total requestBucket
	bucketFor := func(m map[string]*requestBucket, key string) *requestBucket {
		if current, ok := m[key]; ok {
			return current
		}
		created := &requestBucket{}
		m[key] = created
		return created
	}

	for rows.Next() {
		var group modelGroup
		var scenario, day string
		var b requestBucket
		if err := rows.Scan(&group.model, &group.provider, &scenario, &day,
			&b.requests, &b.successes, &b.successRows,
			&b.input, &b.output, &b.cacheRead, &b.cacheCreation, &b.tokens,
			&b.cost, &b.costRows); err != nil {
			return nil, err
		}
		total.add(b)
		if current, ok := models[group]; ok {
			current.add(b)
		} else {
			created := b
			models[group] = &created
		}
		bucketFor(providers, group.provider).add(b)
		bucketFor(scenarios, scenario).add(b)
		bucketFor(days, day).add(b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := &RequestSummary{
		TotalRequests:       total.requests,
		SuccessRows:         total.successRows,
		TotalTokens:         total.tokens,
		InputTokens:         total.input,
		OutputTokens:        total.output,
		CacheReadTokens:     total.cacheRead,
		CacheCreationTokens: total.cacheCreation,
		CostUSD:             total.cost,
		CostRows:            total.costRows,
	}
	if out.SuccessRows > 0 {
		out.SuccessRate = float64(total.successes) / float64(out.SuccessRows)
	}
	out.UnknownCostRequests = out.TotalRequests - out.CostRows

	// The orderings reproduce the ORDER BY each query used to carry: the ranked
	// dimensions by volume then name, the trend by date.
	modelKeys := make([]modelGroup, 0, len(models))
	for key := range models {
		modelKeys = append(modelKeys, key)
	}
	sort.Slice(modelKeys, func(i, j int) bool {
		left, right := models[modelKeys[i]], models[modelKeys[j]]
		if left.requests != right.requests {
			return left.requests > right.requests
		}
		if modelKeys[i].model != modelKeys[j].model {
			return modelKeys[i].model < modelKeys[j].model
		}
		return modelKeys[i].provider < modelKeys[j].provider
	})
	for _, key := range modelKeys {
		b := models[key]
		out.Models = append(out.Models, RequestBreakdown{
			Name: key.model, Provider: key.provider, Requests: b.requests, Tokens: b.tokens,
			CostUSD: b.cost, UnknownCostRequests: b.requests - b.costRows,
		})
	}
	for _, spec := range []struct {
		buckets map[string]*requestBucket
		into    *[]RequestBreakdown
	}{
		{providers, &out.Providers}, {scenarios, &out.Scenarios},
	} {
		keys := make([]string, 0, len(spec.buckets))
		for key := range spec.buckets {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			left, right := spec.buckets[keys[i]], spec.buckets[keys[j]]
			if left.requests != right.requests {
				return left.requests > right.requests
			}
			return keys[i] < keys[j]
		})
		for _, key := range keys {
			b := spec.buckets[key]
			*spec.into = append(*spec.into, RequestBreakdown{
				Name: key, Requests: b.requests, Tokens: b.tokens,
				CostUSD: b.cost, UnknownCostRequests: b.requests - b.costRows,
			})
		}
	}
	daysList := make([]string, 0, len(days))
	for day := range days {
		daysList = append(daysList, day)
	}
	sort.Strings(daysList)
	for _, day := range daysList {
		b := days[day]
		out.Trend = append(out.Trend, RequestTrend{
			Date: day, Requests: b.requests, Tokens: b.tokens,
			CostUSD: b.cost, UnknownCostRequests: b.requests - b.costRows,
		})
	}
	return out, nil
}

// Query returns one server-side filtered history page and the matching total.
func (r *Requests) Query(q RequestQuery) ([]history.RequestRecord, int64, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 1000 {
		q.PageSize = 50
	}
	offset := (q.Page - 1) * q.PageSize
	where, args := requestWhere(q)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var total int64
	if err := r.db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM requests`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := "DESC"
	if strings.EqualFold(q.SortOrder, "asc") {
		order = "ASC"
	}
	sortColumn := requestSortColumn(q.SortBy)
	orderBy := sortColumn + " " + order
	if sortColumn != "julianday(start_time)" {
		orderBy += ", julianday(start_time) DESC"
	}
	orderBy += ", id ASC"

	selectArgs := append(append([]any{}, args...), q.PageSize, offset)
	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT id, model, requested_model, provider, scenario, start_time, duration_ms,
		       input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, reasoning_tokens,
		       cost_usd, cost_source, details_known, streaming, success, error_msg, attempt,
		       peak_multiplier, upstream_channel, upstream_model, first_byte_ms, reasoning_effort,
		       key_hint
		FROM requests`+where+`
		ORDER BY `+orderBy+`
		LIMIT ? OFFSET ?
	`, selectArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	records, err := scanRequests(rows)
	if err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// ObservedChannels lists the upstream channels this history actually recorded,
// most-used first.
//
// It reads the recorded values rather than the gateway's advertised candidate
// list. Those answer different questions: the catalogue says which channels
// exist, and this says which ones served traffic here. Only the second is a
// usable filter - offering a channel that never served a request produces an
// empty page, which reads as a broken filter rather than an unused channel.
//
// provider narrows the list to one platform's rows. It is empty for "every
// platform", which is the same population the history filter sees when no
// platform is selected.
func (r *Requests) ObservedChannels(provider string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT upstream_channel FROM requests WHERE upstream_channel IS NOT NULL`
	var args []any
	if value := strings.TrimSpace(provider); value != "" {
		query += ` AND provider = ?`
		args = append(args, value)
	}
	query += ` GROUP BY upstream_channel ORDER BY COUNT(*) DESC, upstream_channel`

	rows, err := r.db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	channels := []string{}
	for rows.Next() {
		var channel string
		if err := rows.Scan(&channel); err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func requestWhere(q RequestQuery) (string, []any) {
	var clauses []string
	var args []any

	if search := strings.TrimSpace(q.Search); search != "" {
		term := "%" + strings.ToLower(search) + "%"
		// LIKE is case-insensitive for ASCII on its own, so wrapping every
		// column in LOWER() bought nothing and cost roughly half the search
		// time - the function call defeats the comparison's own fast path. The
		// columns hold ASCII model, provider and scenario names and generated
		// UUIDs; there is no non-ASCII text here for LOWER to fold. Verified
		// row for row on a production history: the two forms agree on every
		// row, and a mixed-case term returns the same count.
		//
		// requested_model is searched: "find the requests where the client
		// asked for opus" is the question this column exists to answer, and
		// without it here the term would only ever match the served model.
		//
		// error_msg stays searched even though it is empty on this instance:
		// only the non-streaming failure path fills it, and a history with no
		// such failure yet is not a history that never has one. Dropping the
		// column would make "search the error text" silently match nothing.
		clauses = append(clauses, `(id LIKE ? OR model LIKE ? OR COALESCE(requested_model, '') LIKE ? OR COALESCE(provider, '') LIKE ? OR COALESCE(scenario, '') LIKE ? OR COALESCE(error_msg, '') LIKE ?)`)
		args = append(args, term, term, term, term, term, term)
	}
	for _, filter := range []struct {
		column string
		value  string
	}{
		{"model", q.Model},
		{"provider", q.Provider},
		{"scenario", q.Scenario},
		{"upstream_channel", q.Channel},
	} {
		if value := strings.TrimSpace(filter.value); value != "" {
			clauses = append(clauses, filter.column+" = ?")
			args = append(args, value)
		}
	}
	if q.CostSource == CostSourceProvider || q.CostSource == CostSourceEstimated {
		clauses = append(clauses, "cost_source = ?")
		args = append(args, q.CostSource)
	}
	if q.Start != nil {
		clauses = append(clauses, "julianday(start_time) >= julianday(?)")
		args = append(args, q.Start.UTC().Format(time.RFC3339Nano))
	}
	if q.End != nil {
		clauses = append(clauses, "julianday(start_time) < julianday(?)")
		args = append(args, q.End.UTC().Format(time.RFC3339Nano))
	}
	if q.Success != nil {
		clauses = append(clauses, "details_known = 1 AND success = ?")
		args = append(args, boolToInt(*q.Success))
	}
	if q.Streaming != nil {
		clauses = append(clauses, "details_known = 1 AND streaming = ?")
		args = append(args, boolToInt(*q.Streaming))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func requestSortColumn(field string) string {
	switch field {
	case "model":
		return "LOWER(model)"
	case "provider":
		return "LOWER(COALESCE(provider, ''))"
	case "scenario":
		return "LOWER(COALESCE(scenario, ''))"
	case "total_tokens":
		// The column the history table labels "Tokens" shows the four-part sum,
		// output included. Sorting it by prompt_tokens - the three-part input
		// figure - made the header and the cell disagree: a row with a small
		// prompt and a large answer displayed a big total and sorted near the
		// bottom. The column is sorted by what it displays.
		return "COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0) + COALESCE(cache_read_tokens, 0) + COALESCE(cache_creation_tokens, 0)"
	case "prompt_tokens":
		return "COALESCE(input_tokens, 0) + COALESCE(cache_read_tokens, 0) + COALESCE(cache_creation_tokens, 0)"
	case "output_tokens":
		return "COALESCE(output_tokens, 0)"
	case "cost_usd":
		return "COALESCE(cost_usd, 0)"
	case "reasoning_effort":
		// Pin missing observations last independently of the value direction.
		return "(CASE WHEN COALESCE(reasoning_effort, '') = '' THEN 1 ELSE 0 END) ASC, LOWER(reasoning_effort)"
	case "first_byte_ms":
		return "(CASE WHEN first_byte_ms IS NULL THEN 1 ELSE 0 END) ASC, first_byte_ms"
	case "duration_ms":
		return "COALESCE(duration_ms, 0)"
	case "tokens_per_second":
		// The rate is output over seconds, and a row without both figures has no
		// rate at all: the cell shows a dash, not a zero.
		//
		// The measurable test must match the cell's own condition exactly. The
		// cell renders a rate only when the row has details, succeeded, and has
		// both figures - a failed request's output is a partial count, so its rate
		// is not comparable. A key that omitted either flag would rank rows the
		// column shows as blank, so the reader would see dashes in the middle of
		// an ordered list. Kept in the same order as historyHasDetails in app.js
		// so the two can be read side by side.
		const measurable = "details_known = 1 AND success = 1 " +
			"AND COALESCE(output_tokens, 0) > 0 AND COALESCE(duration_ms, 0) > 0"
		//
		// SQLite sorts NULL before every value, so a bare NULL would leave the
		// unmeasurable rows at the top of an ascending sort - the user asking for
		// the slowest requests would get a screen of dashes first. The leading term
		// pins them to the bottom in both directions.
		//
		// Integer division would truncate to 0 for any request under a second per
		// token, so the numerator is cast to REAL before dividing.
		return "(CASE WHEN " + measurable + " THEN 0 ELSE 1 END) ASC, " +
			"CASE WHEN " + measurable +
			" THEN CAST(COALESCE(output_tokens, 0) AS REAL) / (COALESCE(duration_ms, 0) / 1000.0) " +
			"ELSE NULL END"
	case "success":
		return "COALESCE(success, 0)"
	case "streaming":
		return "COALESCE(streaming, 0)"
	default:
		return "julianday(start_time)"
	}
}

// Totals is a persisted roll-up of request counters, used to backfill the
// overview page after a restart (the in-process metrics counters start at zero).
type Totals struct {
	Received    int64
	Streamed    int64
	Success     int64
	Failed      int64
	ModelCounts map[string]int64
}

// Totals returns lifetime request counters aggregated from the requests table.
func (r *Requests) Totals() (*Totals, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t := &Totals{ModelCounts: map[string]int64{}}
	row := r.db.DB().QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN details_known = 1 AND streaming = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN details_known = 1 AND success = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN details_known = 1 AND success = 0 THEN 1 ELSE 0 END), 0)
		FROM requests
	`)
	if err := row.Scan(&t.Received, &t.Streamed, &t.Success, &t.Failed); err != nil {
		return nil, err
	}

	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT model, COUNT(*) FROM requests GROUP BY model
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var model string
		var n int64
		if err := rows.Scan(&model, &n); err != nil {
			return nil, err
		}
		t.ModelCounts[model] = n
	}
	return t, rows.Err()
}

func scanRequests(rows *sql.Rows) ([]history.RequestRecord, error) {
	var records []history.RequestRecord
	for rows.Next() {
		var rec history.RequestRecord
		var startTimeStr string
		// Imported official rows carry NULL streaming/success/details_known;
		// scan them as nullable booleans instead of failing on NULL.
		var detailsKnown, streaming, success sql.NullInt64

		var attempt sql.NullInt64
		var costUSD sql.NullFloat64
		var costSource sql.NullString
		var errorMsg sql.NullString
		var provider, scenario, requestedModel sql.NullString
		var upstreamChannel, upstreamModel sql.NullString
		var firstByteMS sql.NullInt64
		var reasoningEffort, keyHint sql.NullString
		var peakMul float64
		err := rows.Scan(
			&rec.ID,
			&rec.Model,
			&requestedModel,
			&provider,
			&scenario,
			&startTimeStr,
			&rec.Duration,
			&rec.InputTokens,
			&rec.OutputTokens,
			&rec.CacheReadTokens,
			&rec.CacheCreationTokens,
			&rec.ReasoningTokens,
			&costUSD,
			&costSource,
			&detailsKnown,
			&streaming,
			&success,
			&errorMsg,
			&attempt,
			&peakMul,
			&upstreamChannel,
			&upstreamModel,
			&firstByteMS,
			&reasoningEffort,
			&keyHint,
		)
		if err != nil {
			return nil, err
		}
		rec.Provider = provider.String
		rec.Scenario = scenario.String
		rec.RequestedModel = requestedModel.String
		rec.UpstreamChannel = upstreamChannel.String
		rec.UpstreamModel = upstreamModel.String
		rec.FirstByte = time.Duration(firstByteMS.Int64) * time.Millisecond
		rec.ReasoningEffort = reasoningEffort.String
		rec.KeyHint = keyHint.String
		rec.PeakMultiplier = peakMul
		if attempt.Valid {
			rec.Attempt = int(attempt.Int64)
		} else {
			rec.Attempt = 1
		}
		if costUSD.Valid {
			rec.CostUSD = costUSD.Float64
			rec.CostKnown = true
			if costSource.Valid {
				rec.CostSource = costSource.String
			} else {
				rec.CostSource = CostSourceEstimated
			}
		}
		if errorMsg.Valid {
			rec.ErrorMsg = errorMsg.String
		}

		rec.StartTime, _ = time.Parse(time.RFC3339Nano, startTimeStr)
		rec.StartTime = rec.StartTime.UTC()
		rec.Streaming = streaming.Valid && streaming.Int64 == 1
		rec.Success = success.Valid && success.Int64 == 1
		// DetailsKnown gates the observed status/metadata group, not token or cost data.
		rec.DetailsKnown = detailsKnown.Valid && detailsKnown.Int64 == 1 &&
			success.Valid && (success.Int64 == 0 || success.Int64 == 1)
		rec.Duration = time.Duration(rec.Duration) * time.Millisecond

		records = append(records, rec)
	}

	return records, rows.Err()
}

func (r *Requests) costForRecord(ctx context.Context, rec history.RequestRecord) (sql.NullFloat64, string, error) {
	// A row whose usage was never observed has no cost: pricing zero tokens
	// yields zero, and a stored 0 is indistinguishable from a genuinely free
	// request. NULL is the honest answer, and it is what makes the row land in
	// the unknown-cost bucket rather than in every total that sums cost.
	if rec.UsageUnobserved {
		return sql.NullFloat64{}, "", nil
	}
	if rec.CostKnown || rec.CostUSD != 0 {
		source := rec.CostSource
		if source != CostSourceProvider {
			source = CostSourceEstimated
		}
		return sql.NullFloat64{Float64: rec.CostUSD, Valid: true}, source, nil
	}
	provider := rec.Provider
	if provider == "" {
		provider = "opencode-go"
	}
	var modelsInputPerM, modelsOutputPerM sql.NullFloat64
	var tiersJSON sql.NullString
	err := r.db.DB().QueryRowContext(ctx, `
		SELECT cost_input_per_m, cost_output_per_m, cost_tiers
		FROM models
		WHERE provider = ? AND name = ?
	`, provider, modelNameForPricing(provider, rec.Model)).Scan(&modelsInputPerM, &modelsOutputPerM, &tiersJSON)
	if err != nil && err != sql.ErrNoRows {
		return sql.NullFloat64{}, "", err
	}
	reqTime := rec.StartTime
	if reqTime.IsZero() {
		reqTime = time.Now()
	}
	cost, known := costForProviderTokensAt(rec.Provider, rec.Model,
		int64(rec.InputTokens),
		int64(rec.OutputTokens),
		int64(rec.CacheReadTokens),
		int64(rec.CacheCreationTokens),
		modelsInputPerM,
		modelsOutputPerM,
		reqTime, decodeTiers(tiersJSON))
	if !known {
		return sql.NullFloat64{}, "", nil
	}
	return sql.NullFloat64{Float64: cost, Valid: true}, CostSourceEstimated, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// peakMultiplierForRecord returns the billing multiplier to persist for a
// record: the caller-set value when present, else computed from start time.
// Which platforms peak-price which models is history.ProviderPeakMultiplier's
// decision alone - a second provider gate here would silently disagree with it
// as soon as another platform adopts peak pricing.
func peakMultiplierForRecord(rec history.RequestRecord) float64 {
	if rec.PeakMultiplier > 0 {
		return rec.PeakMultiplier
	}
	return history.ProviderPeakMultiplier(rec.Provider, rec.Model, rec.StartTime)
}
