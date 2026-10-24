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

func TestHistoryExposesRecordedReasoning(t *testing.T) {
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "history.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, record := range []history.RequestRecord{
		{ID: "observed", Provider: "workbuddy", OutputTokens: 36, ReasoningTokens: 26},
		{ID: "unobserved", Provider: "workbuddy", OutputTokens: 36},
	} {
		record.StartTime = time.Now().UTC()
		if err := storage.NewRequests(db).Insert(record); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	(&Server{storage: db}).handleHistory(w, httptest.NewRequest("GET", "/api/history?provider=workbuddy", nil))
	if w.Code != 200 {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Items []struct {
			ID        string `json:"id"`
			Output    int    `json:"output_tokens"`
			Reasoning *int   `json:"reasoning_tokens"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 2 {
		t.Fatalf("history rows = %d, want 2", len(response.Items))
	}
	for _, row := range response.Items {
		if row.Output != 36 {
			t.Errorf("%s output = %d, want 36 without double counting", row.ID, row.Output)
		}
		if row.ID == "observed" && (row.Reasoning == nil || *row.Reasoning != 26) {
			t.Errorf("recorded reasoning missing from history JSON: %+v", row)
		}
		if row.ID == "unobserved" && row.Reasoning != nil {
			t.Errorf("unknown split must remain omitted: %+v", row)
		}
	}
}
