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

func TestHistoryTimingAPIKeepsUnknownAndMeasuredZeroDistinct(t *testing.T) {
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "timing.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	zero, content, end := int64(0), int64(4000), int64(14000)
	for _, id := range []string{"measured", "unknown", "failed", "nonstream"} {
		r := history.RequestRecord{ID: id, Model: "synthetic", Provider: "workbuddy", StartTime: time.Now(), Duration: 16 * time.Second, OutputTokens: 600, ReasoningTokens: 100, Success: id != "failed", Streaming: id != "nonstream"}
		if id != "unknown" && id != "nonstream" {
			r.FirstEventMs = &zero
			r.FirstContentMs = &content
			r.MessageEndMs = &end
		}
		if err := storage.NewRequests(db).Insert(r); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	(&Server{storage: db}).handleHistory(w, httptest.NewRequest("GET", "/api/history", nil))
	if w.Code != 200 {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 4 {
		t.Fatalf("rows=%d", len(body.Items))
	}
	for _, r := range body.Items {
		id := r["id"]
		if id == "measured" {
			for key, want := range map[string]float64{"first_event_ms": 0, "first_content_ms": 4000, "message_end_ms": 14000, "content_tokens_per_second": 60, "event_tokens_per_second": 37.5, "output_tokens": 600, "reasoning_tokens": 100, "duration_ms": 16000} {
				if got, ok := r[key]; !ok || got != want {
					t.Errorf("%s=%v want %v", key, got, want)
				}
			}
		} else {
			for _, key := range []string{"content_tokens_per_second", "event_tokens_per_second"} {
				if _, ok := r[key]; ok {
					t.Errorf("%s has fabricated %s", id, key)
				}
			}
			if id != "failed" {
				for _, key := range []string{"first_event_ms", "first_content_ms", "message_end_ms"} {
					if _, ok := r[key]; ok {
						t.Errorf("%s has fabricated %s", id, key)
					}
				}
			}
		}
	}
}
