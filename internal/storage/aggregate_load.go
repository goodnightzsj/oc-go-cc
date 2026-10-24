package storage

import "context"

// lockAggregate includes time waiting behind another load in the query budget.
func (d *Database) lockAggregate(ctx context.Context) error {
	select {
	case d.aggregateLoad <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
