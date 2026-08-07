package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/routatic/proxy/internal/history"
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

	_, err := r.db.DB().ExecContext(ctx, `
		INSERT OR REPLACE INTO requests (
			id, model, provider, scenario, start_time, duration_ms,
			input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
			streaming, success, error_msg, attempt
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		       streaming, success, error_msg, attempt
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
		       streaming, success, error_msg, attempt
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

// Page returns records for a single history page (1-based page, pageSize per
// page, newest first) plus the total number of records.
func (r *Requests) Page(page, pageSize int) ([]history.RequestRecord, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.db.DB().QueryContext(ctx, `
		SELECT id, model, provider, scenario, start_time, duration_ms,
		       input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
		       streaming, success, error_msg, attempt
		FROM requests
		ORDER BY start_time DESC
		LIMIT ? OFFSET ?
	`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	records, err := scanRequests(rows)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.Count()
	if err != nil {
		return records, 0, err
	}
	return records, total, nil
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
			COALESCE(SUM(CASE WHEN streaming = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END), 0)
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
		var streaming, success int

		var attempt sql.NullInt64
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
			&streaming,
			&success,
			&errorMsg,
			&attempt,
		)
		if attempt.Valid {
			rec.Attempt = int(attempt.Int64)
		} else {
			rec.Attempt = 1
		}
		if errorMsg.Valid {
			rec.ErrorMsg = errorMsg.String
		}
		if err != nil {
			return nil, err
		}

		rec.StartTime, _ = time.Parse(time.RFC3339Nano, startTimeStr)
		rec.Streaming = streaming == 1
		rec.Success = success == 1
		rec.Duration = time.Duration(rec.Duration) * time.Millisecond

		records = append(records, rec)
	}

	return records, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
