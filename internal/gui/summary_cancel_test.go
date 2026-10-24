package gui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

func TestSummaryHandlersHonorCancellation(t *testing.T) {
	for _, name := range []string{"history", "analytics", "daily", "hourly", "performance", "performance-aggregate"} {
		for _, phase := range []string{"before", "waiting"} {
			t.Run(name+"/"+phase, func(t *testing.T) {
				db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "summary.db"), WALEnabled: true})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				s, analytics := &Server{storage: db}, NewAnalyticsHandler(db)
				handler := map[string]http.HandlerFunc{
					"history": s.handleHistorySummary, "analytics": analytics.Summary,
					"daily": analytics.TokenTrend, "hourly": analytics.TokenTrend,
					"performance": s.handlePerformance, "performance-aggregate": s.handlePerformanceAggregate,
				}[name]
				url := "/?days=7&compare=1"
				if name == "hourly" {
					url += "&granularity=hour"
				}
				conn, err := db.DB().Conn(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if phase == "before" {
					cancel()
				}
				w := httptest.NewRecorder()
				done := make(chan struct{})
				go func() {
					defer close(done)
					handler(w, httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx))
				}()
				if phase == "waiting" {
					waitForSummaryDBWait(t, db)
				}
				cancel()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("canceled handler kept waiting for the database")
				}
				if err := conn.Close(); err != nil {
					t.Fatal(err)
				}
				<-done
				if w.Code < 400 {
					t.Errorf("canceled query reported success: HTTP %d", w.Code)
				}
				healthy := httptest.NewRecorder()
				handler(healthy, httptest.NewRequest(http.MethodGet, url, nil))
				if healthy.Code != http.StatusOK {
					t.Fatalf("independent request after cancellation: HTTP %d: %s", healthy.Code, healthy.Body.String())
				}
			})
		}
	}
}

func waitForSummaryDBWait(t *testing.T, db *storage.Database) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for db.DB().Stats().WaitCount == 0 {
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Error("request never reached the occupied connection")
			return
		}
	}
}

func TestCanceledQuotaLedgerIsNotCached(t *testing.T) {
	for _, provider := range []string{"opencode-go", "commandcode"} {
		t.Run(provider, func(t *testing.T) {
			var srv *Server
			var db *storage.Database
			if provider == "commandcode" {
				srv, db = commandCodeLedgerServer(t, `["synthetic-command-key"]`)
			} else {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/docs" {
						_, _ = io.WriteString(w, docsFixture)
					} else {
						_, _ = io.WriteString(w, `{"monthly":{"usedPercent":1,"resetsAt":"2026-10-26T00:00:00Z"}}`)
					}
				}))
				t.Cleanup(upstream.Close)
				var err error
				db, err = storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "quota.db")})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				srv = quotaTestServer(t, upstream.URL, "synthetic-go-key")
				srv.storage = db
				srv.modelLimitsURL = []string{upstream.URL + "/docs"}
			}
			conn, err := db.DB().Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				srv.handleQuota(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/quota?provider="+provider, nil).WithContext(ctx))
			}()
			waitForSummaryDBWait(t, db)
			cancel()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("quota ledger ignored request cancellation")
			}
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			<-done
			if len(srv.quotaCache) != 0 {
				t.Fatal("canceled local ledger was cached as an account result")
			}
			if got := getQuota(t, srv, "?provider="+provider); got.Cached || got.ModelUsageError != "" {
				t.Fatalf("next caller inherited canceled ledger: cached=%t error=%q", got.Cached, got.ModelUsageError)
			}
		})
	}
}
