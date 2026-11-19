package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/pkg/types"
)

func TestEarlyFailureHistorySurvivesSuccessfulFallback(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", path, stream), func(t *testing.T) {
				var calls atomic.Int32
				h, db := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var req struct{ Model string }
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
						return
					}
					if req.Model == "primary" {
						w.WriteHeader(503)
						_, _ = io.WriteString(w, `{"error":{"message":"synthetic overload"}}`)
						return
					}
					if req.Model != "secondary" {
						t.Errorf("unexpected wire model %q", req.Model)
					}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7}}\n\ndata: [DONE]\n\n")
						return
					}
					_, _ = io.WriteString(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`)
				}))
				skipped := config.ModelConfig{Provider: "commandcode", ModelID: "skipped", WireFormat: "openai", MaxTokens: 100}
				fallbacks := []config.ModelConfig{
					{Provider: "commandcode", ModelID: "primary", WireFormat: "openai", MaxTokens: 100},
					{Provider: "commandcode", ModelID: "secondary", WireFormat: "openai", MaxTokens: 100},
				}
				h.modelRouter = router.NewModelRouter(config.NewAtomicConfig(&config.Config{
					Models: map[string]config.ModelConfig{"default": skipped}, RespectRequestedModel: boolPtr(false),
					Fallbacks: map[string][]config.ModelConfig{"default": fallbacks, "fast": fallbacks},
				}, ""))
				for range 3 {
					h.fallbackHandler.AllowAttempt(skipped)(t.Context(), &client.APIError{StatusCode: 503})
				}
				payload := fmt.Sprintf(`{"model":"client-model","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
				if path == "/v1/responses" {
					payload = fmt.Sprintf(`{"model":"client-model","input":"hi","stream":%t}`, stream)
				}
				w := httptest.NewRecorder()
				r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
				if path == "/v1/responses" {
					h.HandleResponses(w, r)
				} else {
					h.HandleMessages(w, r)
				}
				if w.Code != 200 || !strings.Contains(w.Body.String(), "answer") || calls.Load() != 2 {
					t.Fatalf("fallback result status=%d calls=%d body=%s", w.Code, calls.Load(), w.Body.String())
				}
				rows, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
				if err != nil || total != 2 || rows[0].ID == rows[1].ID {
					t.Fatalf("attempts lost or duplicated: total=%d err=%v rows=%+v", total, err, rows)
				}
				for _, row := range rows {
					if row.Model == "primary" {
						if row.Success || row.Attempt != 1 || row.CandidatePosition != 2 || row.CostKnown || row.DetailsKnown || !strings.Contains(row.ErrorMsg, "synthetic overload") {
							t.Fatalf("early failure misrecorded: %+v", row)
						}
					} else if row.Model != "secondary" || !row.Success || row.Attempt != 2 || row.CandidatePosition != 3 || row.OutputTokens != 7 {
						t.Fatalf("successful retry misrecorded: %+v", row)
					}
				}
				metrics := h.metrics.GetSnapshot()
				if metrics.RequestsSuccess != 1 || metrics.RequestsFailed != 0 || metrics.UpstreamCalls != 2 {
					t.Fatalf("attempt failure changed request outcome: %+v", metrics)
				}
			})
		}
	}
}

func TestEmptyStreamFailuresRecordAttemptsNotRequests(t *testing.T) {
	db, h := newFailureTestHandler(t, &noUsageFailureBody{Reader: strings.NewReader("")})
	w := httptest.NewRecorder()
	h.handleStreaming(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil),
		&types.MessageRequest{Stream: boolPtr(true), MaxTokens: 64},
		[]config.ModelConfig{{Provider: "opencode-zen", ModelID: "first"}, {Provider: "opencode-zen", ModelID: "second"}},
		nil, router.ScenarioDefault, "empty-attempts")
	rows, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil || total != 2 {
		t.Fatalf("empty source attempts lost: total=%d err=%v", total, err)
	}
	for _, row := range rows {
		if row.Success || row.CostKnown || row.DetailsKnown || row.ErrorMsg == "" {
			t.Fatalf("empty failure incorrectly measured: %+v", row)
		}
	}
	snapshot := h.metrics.GetSnapshot()
	if snapshot.RequestsFailed != 1 || snapshot.RequestsSuccess != 0 || !strings.Contains(w.Body.String(), "event: error") {
		t.Fatalf("attempts changed logical outcome: metrics=%+v body=%s", snapshot, w.Body.String())
	}
}
