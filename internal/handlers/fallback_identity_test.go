package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/storage"
)

func TestSkippedPrimaryStillCountsFallback(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", path, stream), func(t *testing.T) {
				var calls atomic.Int32
				h, db := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7}}\n\ndata: [DONE]\n\n")
						return
					}
					_, _ = io.WriteString(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`)
				}))
				primary := config.ModelConfig{Provider: "commandcode", ModelID: "primary", WireFormat: "openai", MaxTokens: 100}
				fallbacks := []config.ModelConfig{{Provider: "commandcode", ModelID: "backup", WireFormat: "openai", MaxTokens: 100}}
				h.modelRouter = router.NewModelRouter(config.NewAtomicConfig(&config.Config{
					Models: map[string]config.ModelConfig{"default": primary}, RespectRequestedModel: boolPtr(false),
					Fallbacks: map[string][]config.ModelConfig{"default": fallbacks, "fast": fallbacks},
				}, ""))
				h.fallbackHandler = router.NewFallbackHandler(nil, 3, time.Minute)
				for range 3 {
					h.fallbackHandler.AllowAttempt(primary)(t.Context(), &client.APIError{StatusCode: 503})
				}
				payload := fmt.Sprintf(`{"model":"alias","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
				if path == "/v1/responses" {
					payload = fmt.Sprintf(`{"model":"alias","input":"hi","stream":%t}`, stream)
				}
				w := httptest.NewRecorder()
				r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
				if path == "/v1/responses" {
					h.HandleResponses(w, r)
				} else {
					h.HandleMessages(w, r)
				}
				if calls.Load() != 1 || w.Code != 200 || !strings.Contains(w.Body.String(), "answer") {
					t.Fatalf("backup did not succeed: calls=%d status=%d body=%s", calls.Load(), w.Code, w.Body.String())
				}
				rows, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
				if err != nil || total != 1 || rows[0].Model != "backup" || !rows[0].Success || rows[0].Attempt != 1 || rows[0].CandidatePosition != 2 {
					t.Fatalf("wrong execution history: rows=%+v err=%v", rows, err)
				}
				a := storage.NewAnalytics(db)
				providers, err := a.ProviderBreakdown(t.Context(), a.Window(1))
				if err != nil || len(providers) != 1 {
					t.Fatalf("bad aggregate: providers=%+v err=%v", providers, err)
				}
				if providers[0].FallbackRate != 100 {
					t.Errorf("backup answered first execution: fallback_rate=%v, want 100", providers[0].FallbackRate)
				}
			})
		}
	}
}
