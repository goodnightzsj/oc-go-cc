package storage

import "context"

// lockAggregate includes waiting in the caller's cancellation/timeout budget.
func (d *Database) lockAggregate(ctx context.Context) error {
	select {
	case d.aggregateLoad <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
