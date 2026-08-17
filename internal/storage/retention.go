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
	if days <= 0 {
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

func (r *Retention) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	before := time.Now().AddDate(0, 0, -r.days)

	stats := struct {
		requestsDeleted int64
	}{}

	if requests, err := r.db.DB().ExecContext(ctx, `DELETE FROM requests WHERE created_at < ?`, before.Format(time.RFC3339Nano)); err == nil {
		stats.requestsDeleted, _ = requests.RowsAffected()
	} else {
		slog.Warn("retention cleanup failed", "table", "requests", "error", err)
	}

	if stats.requestsDeleted > 0 {
		slog.Debug("retention cleanup",
			"requests_deleted", stats.requestsDeleted,
			"retention_days", r.days)
	}
}
