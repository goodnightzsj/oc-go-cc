package storage

import (
	"context"
	"strings"
	"time"
)

// RetryOnBusy reruns fn when SQLite reports a lock conflict.
//
// The provider sync and cost reconcile transactions read a large set of rows and
// then write: under WAL a deferred transaction that does this fails with
// SQLITE_BUSY_SNAPSHOT when any other process commits in between, and
// busy_timeout cannot rescue it - the read snapshot is already stale, so
// retrying a statement is meaningless and only a fresh transaction can succeed.
// Those commands run by hand while the proxy is serving, which is exactly when
// the conflict happens.
func RetryOnBusy[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	const attempts = 4
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		value, err := fn()
		if err == nil || !isBusyError(err) || ctx.Err() != nil {
			return value, err
		}
		last = err
		select {
		case <-ctx.Done():
			return value, err
		case <-time.After(time.Duration(attempt+1) * 150 * time.Millisecond):
		}
	}
	var zero T
	return zero, last
}

func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "busy") || strings.Contains(message, "database is locked")
}
