package storage

import (
	"context"
	"log/slog"
	"time"
)

type Retention struct {
	db       *Database
	days     int
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func NewRetention(db *Database, days int) *Retention {
	if days == 0 {
		days = 7
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
	before := time.Now().AddDate(0, 0, -r.days)

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
