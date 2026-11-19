package gui

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/storage"
)

func TestMetricsNeverBackfillLogicalRequestsFromAttempts(t *testing.T) {
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "history.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for i, success := range []bool{false, true} {
		if err := storage.NewRequests(db).Insert(history.RequestRecord{
			ID: []string{"failed-attempt", "successful-fallback"}[i], Model: "model", Provider: "workbuddy",
			StartTime: time.Now(), Duration: time.Second, Success: success, Streaming: true,
			Attempt: i + 1, CandidatePosition: i + 1, UsageUnobserved: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	read := func(m *metrics.Metrics) metricsResponse {
		w := httptest.NewRecorder()
		(&Server{storage: db, met: m}).handleMetrics(w, httptest.NewRequest("GET", "/api/metrics", nil))
		var out metricsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	live := metrics.New()
	live.RecordRequest(true)
	live.RecordSuccess("model", time.Second)
	before := read(live)
	if before.RequestsReceived != 1 || before.RequestsSuccess != 1 || before.RequestsFailed != 0 || before.RequestsStreamed != 1 || before.ModelCounts["model"] != 1 {
		t.Fatalf("live metrics must count the one logical success: %+v", before)
	}
	for _, m := range []*metrics.Metrics{metrics.New(), nil} {
		after := read(m)
		if after.RequestsReceived != 0 || after.RequestsSuccess != 0 || after.RequestsFailed != 0 || after.RequestsStreamed != 0 || len(after.ModelCounts) != 0 {
			t.Errorf("empty process metrics must not become historical attempts: %+v", after)
		}
	}
	_, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{})
	if err != nil || total != 2 {
		t.Fatalf("attempt history must remain available: total=%d err=%v", total, err)
	}
}
