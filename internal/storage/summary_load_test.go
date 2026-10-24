package storage

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestAggregateLoadWaitCanBeCancelled(t *testing.T) {
	db := summaryLoadDB(t, 1)
	if err := db.lockAggregate(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() { <-db.aggregateLoad }()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := db.lockAggregate(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait = %v, want context.Canceled", err)
	}
}

func TestCanceledSummaryWaiterDoesNotCancelOwner(t *testing.T) {
	for _, name := range []string{"history", "analytics", "hourly", "performance"} {
		t.Run(name, func(t *testing.T) {
			db := summaryLoadDB(t, 1)
			if err := db.lockAggregate(t.Context()); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err := summaryLoadQueries(ctx, db)[name]()
			if !errors.Is(err, context.Canceled) {
				t.Errorf("canceled aggregate waiter = %v", err)
			}
			// The canceled waiter must neither consume nor release the owner's gate.
			select {
			case <-db.aggregateLoad:
			default:
				t.Fatal("waiter released another load's gate")
			}
			query := summaryLoadQueries(t.Context(), db)[name]
			first, err := query()
			if err != nil {
				t.Fatal(err)
			}
			second, err := query()
			if err != nil || reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() {
				t.Fatalf("independent load/cache failed after canceled waiter: %v", err)
			}
		})
	}
}

func TestSummaryLoadsReleaseGateAfterQueryFailure(t *testing.T) {
	for _, name := range []string{"history", "analytics", "hourly", "performance"} {
		t.Run(name, func(t *testing.T) {
			db := summaryLoadDB(t, 1)
			query := summaryLoadQueries(t.Context(), db)[name]
			if _, err := db.DB().Exec("ALTER TABLE requests RENAME TO unavailable_requests"); err != nil {
				t.Fatal(err)
			}
			if _, err := query(); err == nil {
				t.Fatal("missing request table must be reported, not cached as empty")
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if err := db.lockAggregate(ctx); err != nil {
				t.Fatalf("query failure leaked the load gate: %v", err)
			}
			<-db.aggregateLoad
			if _, err := db.DB().Exec("ALTER TABLE unavailable_requests RENAME TO requests"); err != nil {
				t.Fatal(err)
			}
			if _, err := query(); err != nil {
				t.Fatalf("retry after storage recovers: %v", err)
			}
		})
	}
}

func summaryLoadDB(t testing.TB, count int) *Database {
	t.Helper()
	db, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "summary.db"), WALEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.DB().Exec(`WITH RECURSIVE n(i) AS (
		SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < ?
	) INSERT INTO requests (id, model, provider, scenario, start_time, duration_ms,
		input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
		cost_usd, details_known, success, attempt)
	SELECT 'load-' || i, CASE WHEN i % 250 = 0 THEN 'rare' ELSE 'common' END,
		CASE WHEN i % 100 = 0 THEN 'workbuddy' ELSE 'cline-pass' END,
		CASE WHEN i % 7 = 0 THEN 'complex' ELSE 'default' END,
		strftime('%Y-%m-%dT%H:%M:%SZ', '2026-10-01', '+' || (i * 30) || ' seconds'),
		1000 + i % 10000, 100, i % 500, 900, 0,
		CASE WHEN i % 11 = 0 THEN NULL ELSE 0.001 END,
		i % 5 != 0, i % 9 != 0, 1 + i % 2 FROM n`, count)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func summaryLoadQueries(ctx context.Context, db *Database) map[string]func() (any, error) {
	a := NewAnalytics(db)
	window, _ := a.WindowBetween(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	return map[string]func() (any, error){
		"history":     func() (any, error) { return NewRequests(db).Summary(ctx, RequestQuery{}) },
		"analytics":   func() (any, error) { return a.Summary(ctx, window) },
		"hourly":      func() (any, error) { return a.TokenTrend(ctx, window, "hour") },
		"performance": func() (any, error) { return NewLatency(db).Snapshot(ctx, time.Time{}) },
	}
}

func TestConcurrentSummaryLoadsShareSnapshot(t *testing.T) {
	db := summaryLoadDB(t, 8000)
	for name, query := range summaryLoadQueries(t.Context(), db) {
		t.Run(name, func(t *testing.T) {
			const readers = 8
			results := make([]any, readers)
			errs := make([]error, readers)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range readers {
				wg.Go(func() { <-start; results[i], errs[i] = query() })
			}
			close(start)
			wg.Wait()
			for i := range readers {
				if errs[i] != nil {
					t.Fatal(errs[i])
				}
				if reflect.ValueOf(results[i]).Pointer() != reflect.ValueOf(results[0]).Pointer() {
					t.Fatalf("concurrent %s readers recomputed the same unchanged snapshot", name)
				}
			}
		})
	}
}

func BenchmarkSummaryLoad(b *testing.B) {
	db := summaryLoadDB(b, 65000)
	for name, query := range summaryLoadQueries(b.Context(), db) {
		for _, mode := range []string{"cold", "cold-parallel", "hot"} {
			b.Run(name+"/"+mode, func(b *testing.B) {
				if _, err := query(); err != nil {
					b.Fatal(err)
				}
				b.ResetTimer()
				for range b.N {
					if mode != "hot" {
						b.StopTimer()
						if _, err := db.DB().Exec("UPDATE history_revision SET revision = revision + 1 WHERE id = 1"); err != nil {
							b.Fatal(err)
						}
						b.StartTimer()
					}
					if mode == "cold-parallel" {
						var wg sync.WaitGroup
						for range 2 {
							wg.Go(func() {
								if _, err := query(); err != nil {
									b.Error(err)
								}
							})
						}
						wg.Wait()
					} else if _, err := query(); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
