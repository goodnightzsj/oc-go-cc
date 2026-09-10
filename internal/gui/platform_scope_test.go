package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

func TestDashboardProviderScope(t *testing.T) {
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "scope.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	providers := []string{"opencode-go", "opencode-zen", "aws-bedrock", "openrouter", "commandcode"}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	for i, provider := range providers {
		for _, known := range []int{0, 1} {
			var cost, success any
			duration := 0
			if known == 1 {
				cost, success, duration = float64(i+1), i%2, (i+1)*100
			}
			if _, err := db.DB().Exec(`INSERT INTO requests
				(id, provider, model, scenario, start_time, input_tokens, output_tokens,
				 cache_read_tokens, cache_creation_tokens, details_known, success, duration_ms, cost_usd)
				VALUES (?, ?, 'org/shared', 'default', ?, ?, 2, 3, 1, ?, ?, ?, ?)`,
				provider+string(rune('a'+known)), provider, stamp, (i+1)*10, known, success, duration, cost); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.DB().Exec(`INSERT INTO requests (id, provider, model, start_time)
		VALUES ('legacy', NULL, 'org/shared', ?)`, stamp); err != nil {
		t.Fatal(err)
	}
	h := NewAnalyticsHandler(db)
	s := &Server{storage: db}
	decode := func(handler http.HandlerFunc, query string, target any) {
		t.Helper()
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest(http.MethodGet, "/?"+query, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: HTTP %d: %s", query, w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), target); err != nil {
			t.Fatal(err)
		}
	}
	for i, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			// The UI sends canonical names; existing underscore aliases stay usable.
			query := "provider=" + url.QueryEscape(strings.ReplaceAll(provider, "-", "_"))
			var result struct {
				Summary    storage.TokenSummary        `json:"summary"`
				Models     []storage.ModelBreakdown    `json:"models"`
				Providers  []storage.ProviderBreakdown `json:"providers"`
				Scenarios  []storage.ScenarioBreakdown `json:"scenarios"`
				LastMinute storage.TokenSummary        `json:"last_minute"`
				Today      storage.TokenSummary        `json:"today"`
				Retained   storage.TokenSummary        `json:"retained"`
			}
			decode(h.Summary, query+"&compare=1", &result)
			for name, summary := range map[string]storage.TokenSummary{
				"summary": result.Summary, "last_minute": result.LastMinute,
				"today": result.Today, "retained": result.Retained,
			} {
				if summary.TotalRequests != 2 || summary.KnownRequests != 1 ||
					summary.InputTokens != int64((i+1)*20) || summary.EstCostUSD != float64(i+1) || summary.UnknownCostRequests != 1 {
					t.Errorf("%s mixed platform data: %+v", name, summary)
				}
			}
			if len(result.Models) != 1 || result.Models[0].Provider != provider || result.Models[0].Requests != 2 ||
				len(result.Providers) != 1 || result.Providers[0].Provider != provider ||
				len(result.Scenarios) != 1 || result.Scenarios[0].Requests != 2 {
				t.Errorf("breakdowns mixed platforms: %+v", result)
			}
			for _, granularity := range []string{"hour", "day"} {
				var result struct {
					Trend []storage.DailyTokenPoint `json:"trend"`
				}
				decode(h.TokenTrend, query+"&granularity="+granularity, &result)
				if len(result.Trend) != 1 || result.Trend[0].Requests != 2 ||
					result.Trend[0].CostUSD != float64(i+1) || result.Trend[0].UnknownCostRequests != 1 {
					t.Errorf("%s trend mixed platform data: %+v", granularity, result)
				}
			}
			var perf []modelPerf
			decode(s.handlePerformance, query+"&range=24h", &perf)
			if len(perf) != 1 || perf[0].Provider != provider || perf[0].Model != "org/shared" ||
				perf[0].Count != 1 || perf[0].AvgMs != int64((i+1)*100) || perf[0].Success+perf[0].Failed != 1 {
				t.Errorf("performance mixed platform data: %+v", perf)
			}
			var aggregate struct {
				TotalRequests int64 `json:"total_requests"`
				AvgLatencyMs  int64 `json:"avg_latency_ms"`
			}
			decode(s.handlePerformanceAggregate, query+"&range=24h", &aggregate)
			if aggregate.TotalRequests != 1 || aggregate.AvgLatencyMs != int64((i+1)*100) {
				t.Errorf("aggregate mixed platform data: %+v", aggregate)
			}
		})
	}
	var all struct {
		Summary storage.TokenSummary `json:"summary"`
	}
	decode(h.Summary, "", &all)
	if all.Summary.TotalRequests != 11 {
		t.Fatalf("all-platform view dropped legacy rows: %+v", all)
	}
	for _, handler := range []http.HandlerFunc{h.Summary, h.TokenTrend, s.handlePerformance, s.handlePerformanceAggregate} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest(http.MethodGet, "/?provider=unrecognized", nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("unsupported provider returned %d, want 400", w.Code)
		}
	}
}

func TestPlatformPerformanceWithoutStorageIsExplicit(t *testing.T) {
	s := &Server{}
	for _, handler := range []http.HandlerFunc{s.handlePerformance, s.handlePerformanceAggregate} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest(http.MethodGet, "/?provider=commandcode", nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("missing platform data source returned %d, want 503", w.Code)
		}
	}
}

func TestEmptyPlatformDashboardCollections(t *testing.T) {
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "empty.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	h := NewAnalyticsHandler(db)
	for _, provider := range []string{"", "opencode-go", "opencode-zen", "aws-bedrock", "openrouter", "commandcode"} {
		t.Run("provider="+provider, func(t *testing.T) {
			for _, endpoint := range []struct {
				handler http.HandlerFunc
				fields  []string
			}{
				{h.Summary, []string{"models", "providers", "scenarios"}},
				{h.TokenTrend, []string{"trend"}},
			} {
				rec := httptest.NewRecorder()
				endpoint.handler(rec, httptest.NewRequest(http.MethodGet, "/?provider="+provider, nil))
				if rec.Code != http.StatusOK {
					t.Fatalf("empty dashboard: HTTP %d: %s", rec.Code, rec.Body.String())
				}
				var response map[string]json.RawMessage
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				for _, field := range endpoint.fields {
					if string(response[field]) != "[]" {
						t.Errorf("empty %s = %s, want []", field, response[field])
					}
				}
			}
		})
	}
}
