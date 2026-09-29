package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/storage"
)

func TestRepeatedCorrelationIDPreservesExecutions(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", path, stream), func(t *testing.T) {
				var calls atomic.Int32
				h, db := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if !stream {
						_, _ = io.WriteString(w, `{"id":"msg_test","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					for _, event := range []string{
						`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"claude-test","content":[],"usage":{"input_tokens":11,"output_tokens":0}}}`,
						`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
						`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"answer"}}`,
						`{"type":"content_block_stop","index":0}`,
						`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`,
						`{"type":"message_stop"}`,
					} {
						_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
					}
				}))
				routeNativeCommandCode(h)
				for range 2 {
					body := fmt.Sprintf(`{"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
					if path == "/v1/responses" {
						body = fmt.Sprintf(`{"model":"m","input":"hi","stream":%t}`, stream)
					}
					r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
					r.Header.Set("X-Request-ID", "reused-correlation-id")
					w := httptest.NewRecorder()
					if path == "/v1/responses" {
						h.HandleResponses(w, r)
					} else {
						h.HandleMessages(w, r)
					}
					if w.Code != http.StatusOK || w.Header().Get("X-Request-ID") != "reused-correlation-id" {
						t.Fatalf("status=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
					}
				}
				rows, total, err := storage.NewRequests(db).Query(storage.RequestQuery{Page: 1, PageSize: 10})
				if err != nil || calls.Load() != 2 || total != 2 {
					t.Fatalf("upstream executions=%d persisted=%d err=%v; want 2 and 2", calls.Load(), total, err)
				}
				if rows[0].ID == rows[1].ID || !rows[0].Success || !rows[1].Success || rows[0].OutputTokens+rows[1].OutputTokens != 14 {
					t.Fatalf("independent accounting lost: %+v", rows)
				}
			})
		}
	}
}
