package gui

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/storage"
)

func TestAnalyticsSummaryUsesPrimaryRequestHistory(t *testing.T) {
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "analytics.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := storage.NewRequests(db).Insert(history.RequestRecord{
		ID: "request-a", StartTime: time.Now().UTC(), Model: "model-a", Provider: "platform-a",
		InputTokens: 10, OutputTokens: 2, CacheReadTokens: 30,
		CostKnown: true, CostUSD: 0.00001234, CostSource: storage.CostSourceProvider, Success: true,
	}); err != nil {
		t.Fatalf("insert request: %v", err)
	}
	recorder := httptest.NewRecorder()
	NewAnalyticsHandler(db).Summary(recorder, httptest.NewRequest("GET", "/api/analytics/summary?days=30", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Summary  storage.TokenSummary  `json:"summary"`
		Today    *storage.TokenSummary `json:"today"`
		Retained *storage.TokenSummary `json:"retained"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Summary.TotalRequests != 1 || response.Summary.EstCostUSD != 0.00001234 {
		t.Fatalf("unexpected request summary: %+v", response.Summary)
	}
	compareRecorder := httptest.NewRecorder()
	NewAnalyticsHandler(db).Summary(compareRecorder, httptest.NewRequest("GET", "/api/analytics/summary?days=30&compare=1", nil))
	if compareRecorder.Code != 200 {
		t.Fatalf("comparison status = %d, body = %s", compareRecorder.Code, compareRecorder.Body.String())
	}
	if err := json.Unmarshal(compareRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode comparison response: %v", err)
	}
	if response.Today == nil || response.Retained == nil || response.Today.TotalRequests != 1 || response.Retained.TotalRequests != 1 {
		t.Fatalf("unexpected comparison summaries: today=%+v retained=%+v", response.Today, response.Retained)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response fields: %v", err)
	}
	if _, ok := fields["account"]; ok {
		t.Fatal("analytics response still exposes a separate account data source")
	}
}

func TestAnalyticsExplicitRangeAndHourlyTrend(t *testing.T) {
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "analytics.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = db.Close() }()

	requests := storage.NewRequests(db)
	for _, record := range []history.RequestRecord{
		{
			ID: "inside", StartTime: time.Date(2026, 8, 9, 12, 30, 0, 0, time.UTC),
			Model: "model-a", Provider: "platform-a", InputTokens: 10, OutputTokens: 2,
			CostKnown: true, CostUSD: 0.25, Success: true,
		},
		{
			ID: "outside", StartTime: time.Date(2026, 8, 8, 23, 59, 0, 0, time.UTC),
			Model: "model-b", Provider: "platform-b", InputTokens: 90, OutputTokens: 8,
			CostKnown: true, CostUSD: 1.25, Success: true,
		},
	} {
		if err := requests.Insert(record); err != nil {
			t.Fatalf("insert %s: %v", record.ID, err)
		}
	}

	handler := NewAnalyticsHandler(db)
	rangeQuery := "from=2026-08-09T00:00:00Z&to=2026-08-10T00:00:00Z"
	summaryRecorder := httptest.NewRecorder()
	handler.Summary(summaryRecorder, httptest.NewRequest("GET", "/api/analytics/summary?"+rangeQuery, nil))
	if summaryRecorder.Code != 200 {
		t.Fatalf("summary status = %d, body = %s", summaryRecorder.Code, summaryRecorder.Body.String())
	}
	var summaryResponse struct {
		Summary storage.TokenSummary `json:"summary"`
	}
	if err := json.Unmarshal(summaryRecorder.Body.Bytes(), &summaryResponse); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summaryResponse.Summary.TotalRequests != 1 || summaryResponse.Summary.EstCostUSD != 0.25 {
		t.Fatalf("explicit range included the wrong rows: %+v", summaryResponse.Summary)
	}

	trendRecorder := httptest.NewRecorder()
	handler.TokenTrend(trendRecorder, httptest.NewRequest("GET", "/api/analytics/trend?"+rangeQuery+"&granularity=hour", nil))
	if trendRecorder.Code != 200 {
		t.Fatalf("trend status = %d, body = %s", trendRecorder.Code, trendRecorder.Body.String())
	}
	var trendResponse struct {
		Granularity string                    `json:"granularity"`
		Trend       []storage.DailyTokenPoint `json:"trend"`
	}
	if err := json.Unmarshal(trendRecorder.Body.Bytes(), &trendResponse); err != nil {
		t.Fatalf("decode trend: %v", err)
	}
	if trendResponse.Granularity != "hour" || len(trendResponse.Trend) != 1 || trendResponse.Trend[0].Requests != 1 {
		t.Fatalf("unexpected hourly trend: %+v", trendResponse)
	}
}

func TestAnalyticsRejectsInvalidRangeAndGranularity(t *testing.T) {
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "analytics.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = db.Close() }()
	handler := NewAnalyticsHandler(db)

	for _, target := range []string{
		"/api/analytics/summary?from=2026-08-09T00:00:00Z",
		"/api/analytics/summary?from=2026-08-10T00:00:00Z&to=2026-08-09T00:00:00Z",
		"/api/analytics/summary?from=2026-01-01T00:00:00Z&to=2026-08-10T00:00:00Z",
		"/api/analytics/trend?granularity=week",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("GET", target, nil)
		if request.URL.Path == "/api/analytics/trend" {
			handler.TokenTrend(recorder, request)
		} else {
			handler.Summary(recorder, request)
		}
		if recorder.Code != 400 {
			t.Errorf("%s: status = %d, body = %s", target, recorder.Code, recorder.Body.String())
		}
	}
}
