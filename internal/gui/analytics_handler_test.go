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
		Summary storage.TokenSummary `json:"summary"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Summary.TotalRequests != 1 || response.Summary.EstCostUSD != 0.00001234 {
		t.Fatalf("unexpected request summary: %+v", response.Summary)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response fields: %v", err)
	}
	if _, ok := fields["account"]; ok {
		t.Fatal("analytics response still exposes a separate account data source")
	}
}
