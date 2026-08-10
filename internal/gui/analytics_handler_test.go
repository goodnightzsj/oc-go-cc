package gui

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

func TestAnalyticsSummaryExposesOpenCodeAccountSnapshot(t *testing.T) {
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "analytics.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = db.Close() }()
	now := time.Now().UTC().Truncate(time.Second)
	if err := db.ReplaceProviderUsage(context.Background(), now, []storage.ProviderCostRecord{{
		Time: now, Model: "model-a", Provider: "platform-a", Plan: "lite",
		InputTokens: 10, OutputTokens: 2, CacheReadTokens: 30, ProviderCostUnits: 1234,
	}}); err != nil {
		t.Fatalf("replace usage: %v", err)
	}
	recorder := httptest.NewRecorder()
	NewAnalyticsHandler(db).Summary(recorder, httptest.NewRequest("GET", "/api/analytics/summary?days=30", nil))
	if recorder.Code != 200 {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Account storage.ProviderUsageAnalytics `json:"account"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Account.Summary.TotalRequests != 1 || response.Account.Summary.CostUSD != 0.00001234 {
		t.Fatalf("unexpected account summary: %+v", response.Account.Summary)
	}
}
