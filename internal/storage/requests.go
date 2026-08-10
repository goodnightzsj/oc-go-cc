package storage

import (
	"context"
	"database/sql"
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
			id, model, provider, scenario, start_time, duration_ms,
			input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
			cost_usd, cost_source, details_known, usage_trusted, streaming, success, error_msg, attempt
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0, ?, ?, ?, ?)
	`,
		rec.ID,
		rec.Model,
		rec.Provider,
		rec.Scenario,
		rec.StartTime.Format(time.RFC3339Nano),
		rec.Duration.Milliseconds(),
		rec.InputTokens,
		rec.OutputTokens,
		rec.CacheReadTokens,
		rec.CacheCreationTokens,
		costUSD,
		costSource,
		boolToInt(rec.Streaming),
		boolToInt(rec.Success),
		rec.ErrorMsg,
		attempt,
	)

	return err
}

const (
	// defaultRequestLimit is the page size used when a caller passes n <= 0.
	defaultRequestLimit = 1000
	// maxRequestScan caps time-range scans. A far-past `since` would otherwise
	// stream the whole table into memory.
	maxRequestScan = 50000
)

// Last returns the most recent n request records ordered by start time.
func (r *Requests) Last(n int) ([]history.RequestRecord, error) {
	if n <= 0 {
		n = defaultRequestLimit
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT id, model, provider, scenario, start_time, duration_ms,
		       input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
		       cost_usd, cost_source, details_known, streaming, success, error_msg, attempt
		FROM requests
		ORDER BY start_time DESC
		LIMIT ?
	`, n)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanRequests(rows)
}

// Since returns request records with start time after the given time, newest
// first, capped at maxRequestScan rows so a wide range cannot exhaust memory.
func (r *Requests) Since(since time.Time) ([]history.RequestRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT id, model, provider, scenario, start_time, duration_ms,
		       input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
		       cost_usd, cost_source, details_known, streaming, success, error_msg, attempt
		FROM requests
		WHERE start_time >= ?
		ORDER BY start_time DESC
		LIMIT ?
	`, since.Format(time.RFC3339Nano), maxRequestScan)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanRequests(rows)
}

// Count returns the total number of request records.
func (r *Requests) Count() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int64
	err := r.db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM requests`).Scan(&count)
	return count, err
}

// RequestQuery describes a filtered, sorted history page. Empty fields keep
// the existing all-records behavior.
type RequestQuery struct {
	Page, PageSize     int
	Search, Model      string
	Provider, Scenario string
	CostSource         string
	Start, End         *time.Time
	Success, Streaming *bool
	SortBy, SortOrder  string
}

type RequestSummary struct {
	TotalRequests int64              `json:"total_requests"`
	SuccessRate   float64            `json:"success_rate"`
	SuccessRows   int64              `json:"success_rows"`
	TotalTokens   int64              `json:"total_tokens"`
	CostUSD       float64            `json:"cost_usd"`
	CostRows      int64              `json:"cost_rows"`
	Models        []RequestBreakdown `json:"models"`
	Providers     []RequestBreakdown `json:"providers"`
	Scenarios     []RequestBreakdown `json:"scenarios"`
	Trend         []RequestTrend     `json:"trend"`
}

type RequestBreakdown struct {
	Name     string  `json:"name"`
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
}

type RequestTrend struct {
	Date     string  `json:"date"`
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
}

// Summary aggregates the exact same population as Query, without pagination.
func (r *Requests) Summary(q RequestQuery) (*RequestSummary, error) {
	where, args := requestWhere(q)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out := &RequestSummary{}
	var successes int64
	if err := r.db.DB().QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN details_known = 1 THEN success ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN details_known = 1 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0) +
		                    COALESCE(cache_read_tokens, 0) + COALESCE(cache_creation_tokens, 0)), 0),
		       COALESCE(SUM(cost_usd), 0), COUNT(cost_usd)
		FROM requests`+where, args...).Scan(&out.TotalRequests, &successes, &out.SuccessRows, &out.TotalTokens, &out.CostUSD, &out.CostRows); err != nil {
		return nil, err
	}
	if out.SuccessRows > 0 {
		out.SuccessRate = float64(successes) / float64(out.SuccessRows)
	}
	for _, spec := range []struct {
		column string
		into   *[]RequestBreakdown
	}{
		{"model", &out.Models}, {"provider", &out.Providers}, {"scenario", &out.Scenarios},
	} {
		dimension := spec.column
		if spec.column == "scenario" {
			dimension = "CASE WHEN LOWER(TRIM(COALESCE(scenario, ''))) IN ('', 'unknown') THEN 'override' ELSE scenario END"
		}
		rows, err := r.db.DB().QueryContext(ctx, `
			SELECT `+dimension+`, COUNT(*),
			       COALESCE(SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0) +
			                    COALESCE(cache_read_tokens, 0) + COALESCE(cache_creation_tokens, 0)), 0),
			       COALESCE(SUM(cost_usd), 0)
			FROM requests`+where+` GROUP BY `+dimension+` ORDER BY COUNT(*) DESC, `+dimension, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var item RequestBreakdown
			if err := rows.Scan(&item.Name, &item.Requests, &item.Tokens, &item.CostUSD); err != nil {
				_ = rows.Close()
				return nil, err
			}
			*spec.into = append(*spec.into, item)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT DATE(start_time), COUNT(*),
		       COALESCE(SUM(COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0) +
		                    COALESCE(cache_read_tokens, 0) + COALESCE(cache_creation_tokens, 0)), 0),
		       COALESCE(SUM(cost_usd), 0)
		FROM requests`+where+` GROUP BY DATE(start_time) ORDER BY DATE(start_time)`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var point RequestTrend
		if err := rows.Scan(&point.Date, &point.Requests, &point.Tokens, &point.CostUSD); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out.Trend = append(out.Trend, point)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return out, rows.Close()
}

// Page returns records for a single history page (1-based page, pageSize per
// page, newest first) plus the total number of records.
func (r *Requests) Page(page, pageSize int) ([]history.RequestRecord, int64, error) {
	return r.Query(RequestQuery{Page: page, PageSize: pageSize})
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
	if sortColumn != "start_time" {
		orderBy += ", start_time DESC"
	}

	selectArgs := append(append([]any{}, args...), q.PageSize, offset)
	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT id, model, provider, scenario, start_time, duration_ms,
		       input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
		       cost_usd, cost_source, details_known, streaming, success, error_msg, attempt
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

func requestWhere(q RequestQuery) (string, []any) {
	var clauses []string
	var args []any

	if search := strings.TrimSpace(q.Search); search != "" {
		term := "%" + strings.ToLower(search) + "%"
		clauses = append(clauses, `(LOWER(id) LIKE ? OR LOWER(model) LIKE ? OR LOWER(COALESCE(provider, '')) LIKE ? OR LOWER(COALESCE(scenario, '')) LIKE ? OR LOWER(COALESCE(error_msg, '')) LIKE ?)`)
		args = append(args, term, term, term, term, term)
	}
	for _, filter := range []struct {
		column string
		value  string
	}{
		{"model", q.Model},
		{"provider", q.Provider},
		{"scenario", q.Scenario},
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
	case "prompt_tokens":
		return "COALESCE(input_tokens, 0) + COALESCE(cache_read_tokens, 0) + COALESCE(cache_creation_tokens, 0)"
	case "output_tokens":
		return "COALESCE(output_tokens, 0)"
	case "cost_usd":
		return "COALESCE(cost_usd, 0)"
	case "duration_ms":
		return "COALESCE(duration_ms, 0)"
	case "success":
		return "COALESCE(success, 0)"
	case "streaming":
		return "COALESCE(streaming, 0)"
	default:
		return "start_time"
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

// CountSince returns the number of request records with start time after the given time.
func (r *Requests) CountSince(since time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int64
	err := r.db.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM requests WHERE start_time >= ?
	`, since.Format(time.RFC3339Nano)).Scan(&count)
	return count, err
}

// DeleteBefore removes request records older than the given time.
func (r *Requests) DeleteBefore(before time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := r.db.DB().ExecContext(ctx, `
		DELETE FROM requests WHERE created_at < ?
	`, before.Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func scanRequests(rows *sql.Rows) ([]history.RequestRecord, error) {
	var records []history.RequestRecord
	for rows.Next() {
		var rec history.RequestRecord
		var startTimeStr string
		var detailsKnown, streaming, success int

		var attempt sql.NullInt64
		var costUSD sql.NullFloat64
		var costSource sql.NullString
		var errorMsg sql.NullString
		err := rows.Scan(
			&rec.ID,
			&rec.Model,
			&rec.Provider,
			&rec.Scenario,
			&startTimeStr,
			&rec.Duration,
			&rec.InputTokens,
			&rec.OutputTokens,
			&rec.CacheReadTokens,
			&rec.CacheCreationTokens,
			&costUSD,
			&costSource,
			&detailsKnown,
			&streaming,
			&success,
			&errorMsg,
			&attempt,
		)
		if err != nil {
			return nil, err
		}
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
		rec.Streaming = streaming == 1
		rec.Success = success == 1
		rec.DetailsKnown = detailsKnown == 1
		rec.Duration = time.Duration(rec.Duration) * time.Millisecond

		records = append(records, rec)
	}

	return records, rows.Err()
}

func (r *Requests) costForRecord(ctx context.Context, rec history.RequestRecord) (float64, string, error) {
	if rec.CostKnown || rec.CostUSD != 0 {
		source := rec.CostSource
		if source != CostSourceProvider {
			source = CostSourceEstimated
		}
		return rec.CostUSD, source, nil
	}
	var modelsInputPerM, modelsOutputPerM float64
	err := r.db.DB().QueryRowContext(ctx, `
		SELECT COALESCE(cost_input_per_m, 0), COALESCE(cost_output_per_m, 0)
		FROM models
		WHERE id = ?
	`, rec.Model).Scan(&modelsInputPerM, &modelsOutputPerM)
	if err != nil && err != sql.ErrNoRows {
		return 0, "", err
	}
	return costForTokens(rec.Model,
		int64(rec.InputTokens),
		int64(rec.OutputTokens),
		int64(rec.CacheReadTokens),
		int64(rec.CacheCreationTokens),
		modelsInputPerM,
		modelsOutputPerM), CostSourceEstimated, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
