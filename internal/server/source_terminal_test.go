package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
)

func TestSourceTerminalSemanticsReachBothInbounds(t *testing.T) {
	for _, wire := range []string{"openai", "gemini", "responses"} {
		for _, outcome := range []string{"normal", "budget", "refusal", "error", "truncated"} {
			for _, stream := range []bool{false, true} {
				if outcome == "truncated" && !stream {
					continue
				}
				for _, path := range []string{"/v1/messages", "/v1/responses"} {
					t.Run(fmt.Sprintf("%s/%s/stream=%t%s", wire, outcome, stream, path), func(t *testing.T) {
						var calls atomic.Int32
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							calls.Add(1)
							_, _ = io.Copy(io.Discard, r.Body)
							first, terminal := sourceTerminalFixture(wire, outcome, stream)
							if stream {
								w.Header().Set("Content-Type", "text/event-stream")
								_, _ = fmt.Fprintf(w, "data:%s\r\n\r\n", first)
								if outcome != "truncated" {
									_, _ = fmt.Fprintf(w, "data:%s\r\n\r\n", terminal)
								}
							} else {
								_, _ = io.WriteString(w, first)
							}
						}))
						t.Cleanup(upstream.Close)
						respect := false
						provider := "opencode-zen"
						if wire == "openai" {
							provider = "openrouter"
						}
						cfg := &config.Config{
							OpenRouter:            config.OpenRouterConfig{APIKey: "synthetic", BaseURL: upstream.URL},
							OpenCodeZen:           config.OpenCodeZenConfig{APIKey: "synthetic", GeminiBaseURL: upstream.URL, ResponsesBaseURL: upstream.URL},
							Models:                map[string]config.ModelConfig{"default": {Provider: provider, ModelID: "synthetic", WireFormat: wire}},
							RespectRequestedModel: &respect,
							Storage:               &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
						}
						srv, err := NewServer(config.NewAtomicConfig(cfg, ""), nil)
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = srv.Close() })
						payload := fmt.Sprintf(`{"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
						if path == "/v1/responses" {
							payload = fmt.Sprintf(`{"model":"m","input":"hi","stream":%t}`, stream)
						}
						w := httptest.NewRecorder()
						srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))
						failed := outcome == "error" || outcome == "truncated"
						if failed {
							nativeFailure := wire == "responses" && path == "/v1/responses" && outcome == "error"
							failureMarker := `"error"`
							if nativeFailure {
								failureMarker = `"status":"failed"`
							}
							if strings.Contains(w.Body.String(), `"type":"message_stop"`) || strings.Contains(w.Body.String(), `"type":"response.completed"`) || !strings.Contains(w.Body.String(), failureMarker) {
								t.Fatalf("source failure became success: HTTP %d %s", w.Code, w.Body.String())
							}
							if !stream && !nativeFailure {
								return
							} // Absent non-stream failure usage is covered by the separate accounting contract.
						} else {
							want := map[string]string{"normal": "end_turn", "budget": "max_tokens", "refusal": "refusal"}[outcome]
							if path == "/v1/responses" {
								want = map[string]string{"normal": "completed", "budget": "max_output_tokens", "refusal": "content_filter"}[outcome]
							}
							if w.Code != 200 || !strings.Contains(w.Body.String(), `"`+want+`"`) {
								t.Fatalf("terminal lost: want=%s HTTP %d %s", want, w.Code, w.Body.String())
							}
						}
						if calls.Load() != 1 {
							t.Fatalf("request replayed: %d upstream calls", calls.Load())
						}
						rows, total, err := storage.NewRequests(srv.Storage()).Query(t.Context(), storage.RequestQuery{})
						if err != nil || total != 1 {
							t.Fatalf("history total=%d err=%v", total, err)
						}
						row := rows[0]
						if !row.OutcomeKnown || row.Success == failed || row.InputTokens != 11 || row.OutputTokens != 7 {
							t.Fatalf("outcome/partial usage lost: %+v", row)
						}
					})
				}
			}
		}
	}
}

func sourceTerminalFixture(wire, outcome string, stream bool) (string, string) {
	switch wire {
	case "openai":
		reason := map[string]string{"normal": "stop", "budget": "length", "refusal": "content_filter"}[outcome]
		if stream {
			return `{"choices":[{"delta":{"content":"partial"}}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`, chatTerminal(reason, outcome)
		}
		if outcome == "error" {
			return `{"error":{"type":"server_error","message":"synthetic"}}`, ""
		}
		return fmt.Sprintf(`{"choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":%q}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`, reason), ""
	case "gemini":
		reason := map[string]string{"normal": "STOP", "budget": "MAX_TOKENS", "refusal": "SAFETY", "error": "MALFORMED_FUNCTION_CALL"}[outcome]
		if stream {
			return `{"candidates":[{"content":{"role":"model","parts":[{"text":"partial"}]}}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7}}`, fmt.Sprintf(`{"candidates":[{"finishReason":%q}]}`, reason)
		}
		return fmt.Sprintf(`{"candidates":[{"content":{"role":"model","parts":[{"text":"answer"}]},"finishReason":%q}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7}}`, reason), ""
	default:
		status, details := "completed", ""
		if outcome == "error" {
			status = "failed"
		}
		if outcome == "budget" || outcome == "refusal" {
			status = "incomplete"
			reason := "max_output_tokens"
			if outcome == "refusal" {
				reason = "content_filter"
			}
			details = fmt.Sprintf(`,"incomplete_details":{"reason":%q}`, reason)
		}
		response := fmt.Sprintf(`{"status":%q%s,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":11,"output_tokens":7}}`, status, details)
		if stream {
			return `{"type":"response.output_text.delta","delta":"partial","usage":{"input_tokens":11,"output_tokens":7}}`, fmt.Sprintf(`{"type":"response.%s","response":%s}`, status, response)
		}
		return response, ""
	}
}

func chatTerminal(reason, outcome string) string {
	if outcome == "error" {
		return `{"error":{"type":"server_error","message":"synthetic"}}`
	}
	return fmt.Sprintf(`{"choices":[{"delta":{},"finish_reason":%q}]}`, reason)
}
