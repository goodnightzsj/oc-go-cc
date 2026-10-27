package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
)

func TestSourceUsagePresenceReachesHistory(t *testing.T) {
	for _, wire := range []string{"openai", "responses", "gemini", "anthropic"} {
		for _, usage := range []string{"absent", "zero", "reported"} {
			for _, failed := range []bool{false, true} {
				for _, stream := range []bool{false, true} {
					if wire == "anthropic" && failed && !stream {
						continue // Native non-2xx handling is a separate HTTP contract.
					}
					for _, path := range []string{"/v1/messages", "/v1/responses"} {
						t.Run(fmt.Sprintf("%s/%s/failed=%t/stream=%t%s", wire, usage, failed, stream, path), func(t *testing.T) {
							upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								_, _ = io.Copy(io.Discard, r.Body)
								if stream {
									w.Header().Set("Content-Type", "text/event-stream")
								}
								_, _ = io.WriteString(w, sourceUsageFixture(wire, usage, failed, stream))
							}))
							t.Cleanup(upstream.Close)
							respect := false
							srv, err := NewServer(config.NewAtomicConfig(&config.Config{
								ActiveSite: "opencode-zen",
								OpenCodeZen: config.OpenCodeZenConfig{APIKey: "synthetic", BaseURL: upstream.URL,
									ResponsesBaseURL: upstream.URL, GeminiBaseURL: upstream.URL, AnthropicBaseURL: upstream.URL},
								Models:                map[string]config.ModelConfig{"default": {Provider: "opencode-zen", ModelID: "synthetic", WireFormat: wire, MaxTokens: 100}},
								RespectRequestedModel: &respect,
								Storage:               &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
							}, ""), nil)
							if err != nil {
								t.Fatal(err)
							}
							t.Cleanup(func() { _ = srv.Close() })
							payload := fmt.Sprintf(`{"model":"m","messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
							if path == "/v1/responses" {
								payload = fmt.Sprintf(`{"model":"m","input":"hi","stream":%t}`, stream)
							}
							w := httptest.NewRecorder()
							srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))
							if !failed && w.Code != 200 {
								t.Fatalf("HTTP=%d body=%s", w.Code, w.Body.String())
							}
							rows, total, err := storage.NewRequests(srv.Storage()).Query(t.Context(), storage.RequestQuery{})
							if err != nil || total != 1 {
								t.Fatalf("rows=%d err=%v HTTP=%d body=%s", total, err, w.Code, w.Body.String())
							}
							row := rows[0]
							if !row.OutcomeKnown || row.Success == failed || row.DetailsKnown != (usage != "absent") {
								t.Fatalf("source observation lost: %+v", row)
							}
							if usage == "absent" && row.CostKnown {
								t.Fatal("unreported usage was priced as zero")
							}
							if usage == "absent" && row.GenerationSample != nil {
								t.Fatal("unreported usage became a throughput sample")
							}
							wantInput, wantOutput := 0, 0
							if usage == "reported" {
								wantInput, wantOutput = 11, 7
							}
							if row.InputTokens != wantInput || row.OutputTokens != wantOutput {
								t.Fatalf("reported usage lost: %+v", row)
							}
							if !failed && usage == "absent" {
								for _, token := range []string{`"input_tokens":0`, `"output_tokens":0`} {
									if strings.Contains(w.Body.String(), token) {
										t.Fatalf("downstream synthesized observed zero: %s", w.Body.String())
									}
								}
							}
						})
					}
				}
			}
		}
	}
}

func sourceUsageFixture(wire, usage string, failed, stream bool) string {
	outcome := "normal"
	if failed {
		outcome = "error"
	}
	first, terminal := sourceTerminalFixture(wire, outcome, stream)
	if wire == "anthropic" {
		first = `{"type":"message","role":"assistant","model":"synthetic","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn"}`
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(first), &body); err != nil {
		panic(err)
	}
	key, inputKey, outputKey := "usage", "input_tokens", "output_tokens"
	if wire == "openai" {
		inputKey, outputKey = "prompt_tokens", "completion_tokens"
	} else if wire == "gemini" {
		key, inputKey, outputKey = "usageMetadata", "promptTokenCount", "candidatesTokenCount"
	}
	delete(body, key)
	if usage != "absent" {
		input, output := 0, 0
		if usage == "reported" {
			input, output = 11, 7
		}
		body[key] = map[string]int{inputKey: input, outputKey: output}
	}
	encoded, _ := json.Marshal(body)
	if !stream {
		return string(encoded)
	}
	if wire == "anthropic" {
		start, _ := json.Marshal(map[string]any{"type": "message_start", "message": body})
		end := `{"type":"message_stop"}`
		if failed {
			end = `{"type":"error","error":{"type":"api_error","message":"synthetic"}}`
		}
		return fmt.Sprintf("data:%s\n\ndata:{\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"answer\"}}\n\ndata:{\"type\":\"content_block_stop\",\"index\":0}\n\ndata:{\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\ndata:%s\n\n", start, end)
	}
	// Responses' terminal fixture also carries usage; the first event owns the observation here.
	if wire == "responses" {
		var event map[string]any
		_ = json.Unmarshal([]byte(terminal), &event)
		delete(event["response"].(map[string]any), "usage")
		b, _ := json.Marshal(event)
		terminal = string(b)
	}
	return fmt.Sprintf("data:%s\n\ndata:%s\n\n", encoded, terminal)
}
