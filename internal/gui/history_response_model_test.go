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

func TestHistoryAPIKeepsSourceIdentitiesSeparate(t *testing.T) {
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, row := range []history.RequestRecord{
		{ID: "observed", Model: "cline-pass/route", Provider: "cline-pass", ResponseModel: "vmc/served", UpstreamModel: "deepseek/canonical", UpstreamChannel: "private"},
		{ID: "legacy", Model: "cline-pass/route", Provider: "cline-pass", UpstreamModel: "deepseek/canonical"},
	} {
		row.StartTime = time.Now()
		if err := storage.NewRequests(db).Insert(row); err != nil {
			t.Fatal(err)
		}
	}
	for _, search := range []string{"", "?search=vmc"} {
		w := httptest.NewRecorder()
		(&Server{storage: db}).handleHistory(w, httptest.NewRequest("GET", "/api/history"+search, nil))
		if w.Code != 200 {
			t.Fatalf("HTTP=%d body=%s", w.Code, w.Body.String())
		}
		var page struct {
			Items []map[string]any
			Total int
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		want := 2
		if search != "" {
			want = 1
		}
		if page.Total != want || len(page.Items) != want {
			t.Fatalf("page=%+v", page)
		}
		for _, row := range page.Items {
			if row["model"] != "cline-pass/route" || row["provider"] != "cline-pass" || row["upstream_model"] != "deepseek/canonical" {
				t.Fatalf("attribution changed: %+v", row)
			}
			if row["id"] == "observed" {
				if row["response_model"] != "vmc/served" || row["upstream_channel"] != "private" {
					t.Fatalf("source metadata lost: %+v", row)
				}
			} else if _, ok := row["response_model"]; ok {
				t.Fatalf("legacy response model fabricated: %+v", row)
			}
		}
	}
}
