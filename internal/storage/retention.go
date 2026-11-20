package storage

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

type Retention struct {
	db       *Database
	days     int
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
	// lastRun is the wall clock of the previous pass. Deletion is irreversible
	// and the cutoff is derived from the clock: a forward step would delete more
	// than the window without leaving any trace of why.
	lastRun time.Time
	// now is the clock, replaceable so a test can move it without waiting.
	now func() time.Time
}

func NewRetention(db *Database, days int) *Retention {
	if days == 0 {
		days = DefaultRetentionDays
	}
	return &Retention{
		db:       db,
		days:     days,
		interval: 1 * time.Hour,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

func (r *Retention) Start() {
	go r.run()
}

func (r *Retention) Stop() {
	close(r.stopCh)
	<-r.doneCh
}

// newestRequest returns the latest created_at this table holds. Retention only
// serves to drop old rows, so a table with nothing in it has nothing to bound.
func (r *Retention) newestRequest() (time.Time, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var newest sql.NullString
	if err := r.db.DB().QueryRowContext(ctx, `SELECT MAX(created_at) FROM requests`).Scan(&newest); err != nil || !newest.Valid {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02 15:04:05", newest.String)
	return t, err == nil
}

func (r *Retention) run() {
	defer close(r.doneCh)
	if r.days < 0 {
		slog.Info("request retention cleanup disabled")
		return
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.runOnce()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.runOnce()
		}
	}
}

// retentionBatch bounds one DELETE. Stopping the process waits for the batch in
// flight, so the batch - not the whole cleanup - is what has to be short.
const retentionBatch = 5000

func (r *Retention) runOnce() {
	// UTC() strips the monotonic reading, and that is the point: the guard below
	// exists to notice a wall-clock step, and Sub on two monotonic values walks
	// the monotonic clock, where a step never shows up.
	clock := r.now
	if clock == nil {
		clock = time.Now
	}
	now := clock().UTC()
	if !r.lastRun.IsZero() {
		gap := now.Sub(r.lastRun)
		if gap < 0 || gap > 24*time.Hour {
			slog.Warn("skipping retention cleanup: the clock moved unexpectedly",
				"since_last_run", gap.String(), "retention_days", r.days)
			r.lastRun = now
			return
		}
	}
	r.lastRun = now

	before := now.AddDate(0, 0, -r.days)

	// The in-memory guard above only sees steps that happen while this process
	// runs. A clock that jumps forward while it is stopped - a VM resumed from a
	// snapshot, an RTC reset - arrives here as a perfectly ordinary first run,
	// and the cutoff would sit in the future of every row. Bound it against the
	// newest row this database holds: more than the window ahead of the newest
	// write is a clock that cannot be trusted, not a table that is too old.
	if newest, ok := r.newestRequest(); ok && before.After(newest.AddDate(0, 0, r.days)) {
		slog.Warn("skipping retention cleanup: the clock is ahead of the newest record",
			"cutoff", before.String(), "newest", newest.String(), "retention_days", r.days)
		r.lastRun = now
		return
	}

	stats := struct {
		requestsDeleted int64
	}{}

	// One spelling, so this is a plain indexed comparison. The column's DEFAULT
	// (CURRENT_TIMESTAMP) and every writer use "YYYY-MM-DD HH:MM:SS" UTC; the
	// one-time migration in Open normalizes rows written by older versions in
	// RFC3339Nano. A mixed table made a text comparison delete up to a day
	// more than configured (' ' sorts before 'T'), and forced a full scan.
	// Batched, and checked between batches: a single unbounded DELETE held the
	// only connection for up to a minute and made Ctrl+C or a systemd stop wait
	// for it.
	cutoff := before.UTC().Format("2006-01-02 15:04:05")
	for {
		select {
		case <-r.stopCh:
			return
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		requests, err := r.db.DB().ExecContext(ctx,
			`DELETE FROM requests WHERE rowid IN (SELECT rowid FROM requests WHERE created_at < ? LIMIT ?)`,
			cutoff, retentionBatch)
		cancel()
		if err != nil {
			slog.Warn("retention cleanup failed", "table", "requests", "error", err)
			return
		}
		deleted, _ := requests.RowsAffected()
		stats.requestsDeleted += deleted
		if deleted < retentionBatch {
			break
		}
	}

	if stats.requestsDeleted > 0 {
		slog.Debug("retention cleanup",
			"requests_deleted", stats.requestsDeleted,
			"retention_days", r.days)
	}
}
