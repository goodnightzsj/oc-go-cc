package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
)

func TestNativeTerminalSurvivesBrokenHTTPChunkTrailer(t *testing.T) {
	for _, source := range []string{"anthropic", "responses"} {
		for _, path := range []string{"/v1/messages", "/v1/responses"} {
			if source == "responses" && path == "/v1/messages" {
				continue
			}
			t.Run(source+path, func(t *testing.T) {
				wire := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":5,\"output_tokens\":2}}}\n\n"
				if source == "anthropic" {
					wire = "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":5}}}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\ndata: {\"type\":\"message_stop\"}\n\n"
				}
				var calls atomic.Int32
				srv := nativeResponsesServer(t, "commandcode", func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					defer conn.Close()
					// A complete protocol terminal, but no final HTTP zero chunk.
					_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n", len(wire), wire)
				}, func(cfg *config.Config) {
					if source == "anthropic" {
						cfg.CommandCode.AnthropicBaseURL = cfg.CommandCode.ResponsesBaseURL
						model := cfg.Models["default"]
						model.WireFormat = "anthropic"
						cfg.Models["default"] = model
					}
				})
				payload := `{"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":true}`
				if path == "/v1/responses" {
					payload = `{"model":"m","input":"hi","max_output_tokens":100,"stream":true}`
				}
				w := httptest.NewRecorder()
				srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))
				terminal := `"type":"message_stop"`
				if path == "/v1/responses" {
					terminal = `"type":"response.completed"`
				}
				if w.Code != 200 || calls.Load() != 1 || !strings.Contains(w.Body.String(), terminal) || strings.Contains(w.Body.String(), `"type":"error"`) {
					t.Fatalf("late transport error appended/retried: calls=%d HTTP %d %s", calls.Load(), w.Code, w.Body.String())
				}
				if (source == "responses" || path == "/v1/messages") && w.Body.String() != wire {
					t.Fatal("native bytes changed")
				}
				rows, total, err := storage.NewRequests(srv.Storage()).Query(t.Context(), storage.RequestQuery{})
				if err != nil || total != 1 || !rows[0].Success || rows[0].InputTokens != 5 || rows[0].OutputTokens != 2 {
					t.Fatalf("completion/usage lost: rows=%+v total=%d err=%v", rows, total, err)
				}
			})
		}
	}
}
