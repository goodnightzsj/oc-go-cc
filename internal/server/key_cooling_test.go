package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
)

func TestKeyCoolingReachesBothProtocols(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", path, stream), func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Retry-After", "42")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = io.WriteString(w, `{"error":{"message":"synthetic rate limit"}}`)
				}))
				t.Cleanup(upstream.Close)
				respect := false
				srv, err := NewServer(config.NewAtomicConfig(&config.Config{
					ActiveSite:            "cline-pass",
					ClinePass:             config.ClinePassConfig{APIKeys: []string{"synthetic-a", "synthetic-b"}, BaseURL: upstream.URL},
					Models:                map[string]config.ModelConfig{"default": {Provider: "cline-pass", ModelID: "synthetic", MaxTokens: 100}},
					RespectRequestedModel: &respect,
					Storage:               &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
				}, ""), nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = srv.Close() })
				for i := 0; i < 3; i++ {
					payload := fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hi"}],"max_tokens":100,"stream":%t}`, stream)
					if path == "/v1/responses" {
						payload = fmt.Sprintf(`{"model":"client-alias","input":"hi","stream":%t}`, stream)
					}
					w := httptest.NewRecorder()
					srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(payload)))
					body := w.Body.String()
					if i < 2 {
						if !strings.Contains(body, "synthetic rate limit") || (!stream && w.Code != 429) {
							t.Fatalf("upstream refusal lost: status=%d body=%s", w.Code, body)
						}
						if !stream && w.Header().Get("Retry-After") != "42" {
							t.Fatalf("source retry hint lost: %v", w.Header())
						}
						if stream && (!strings.Contains(body, `"status_code":429`) || !strings.Contains(body, `"retry_after":"42"`)) {
							t.Fatalf("source stream retry hint lost: %s", body)
						}
						continue
					}
					if stream {
						if !strings.Contains(body, `"status_code":503`) || !strings.Contains(body, `"retry_at":`) || !strings.Contains(body, `"retry_after":`) {
							t.Fatalf("structured cooling metadata lost: %s", body)
						}
						// Headers and heartbeat were already committed; failure must be an SSE event.
						failure := "event: error"
						if path == "/v1/responses" {
							failure = "event: response.failed"
						}
						if w.Code != 200 || !strings.Contains(body, failure) || strings.Contains(body, "event: response.completed") || strings.Contains(body, "event: message_stop") {
							t.Fatalf("invalid failed stream: status=%d body=%s", w.Code, body)
						}
					} else {
						seconds, _ := strconv.Atoi(w.Header().Get("Retry-After"))
						if w.Code != 503 || seconds < 1 || seconds > 42 {
							t.Fatalf("cooling status=%d retry=%q body=%s", w.Code, w.Header().Get("Retry-After"), body)
						}
					}
					if !strings.Contains(body, "every API key is resting") || !strings.Contains(body, "retry at") {
						t.Fatalf("cooling reason or retry time missing: %s", body)
					}
				}
				if got := calls.Load(); got != 2 {
					t.Fatalf("cooling pool sent extra requests: %d", got)
				}
				rows, total, err := storage.NewRequests(srv.Storage()).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
				if err != nil || total != 3 {
					t.Fatalf("early failures lost: rows=%d err=%v", total, err)
				}
				for _, row := range rows {
					if row.Success || !row.OutcomeKnown || row.DetailsKnown || row.CostKnown || row.OutputTokens != 0 || row.Attempt != 1 {
						t.Fatalf("invalid early failure accounting: %+v", row)
					}
				}
			})
		}
	}
}
