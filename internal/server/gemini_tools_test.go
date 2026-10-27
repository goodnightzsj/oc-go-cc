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
	"github.com/routatic/proxy/pkg/types"
)

func TestGeminiToolsTwoRoundsThroughBothInbounds(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", path, streaming), func(t *testing.T) {
				calls := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					var req types.GeminiRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Error(err)
						http.Error(w, "bad JSON", 400)
						return
					}
					if r.URL.Path != "/models/gemini-2.5-flash" || req.Stream != streaming || req.SystemInstruction == nil || req.SystemInstruction.Parts[0].Text != "instructions" {
						t.Errorf("wire identity/system lost: %+v %s", req, r.URL.Path)
					}
					if len(req.Contents) < 1 || len(req.Contents[0].Parts) != 2 || req.Contents[0].Parts[1].InlineData == nil || req.Contents[0].Parts[1].InlineData.Data != "YQ==" {
						t.Errorf("image lost: %+v", req.Contents)
					}
					response := `{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}]}`
					if calls == 1 {
						response = `{"candidates":[{"content":{"role":"model","parts":[{"text":"plan","thought":true},{"functionCall":{"id":"a","name":"lookup","args":{"q":1}}},{"functionCall":{"id":"b","name":"lookup","args":{"q":2}}}]},"finishReason":"STOP"}]}`
					} else {
						if len(req.Contents) != 3 {
							t.Errorf("second round content=%+v", req.Contents)
						} else {
							model, result := req.Contents[1], req.Contents[2]
							if model.Role != "model" || len(model.Parts) != 3 || model.Parts[0].Text != "plan" || !model.Parts[0].Thought || model.Parts[1].FunctionCall == nil || model.Parts[1].FunctionCall.ID != "a" || string(model.Parts[1].FunctionCall.Args) != `{"q":1}` {
								t.Errorf("assistant replay lost: %+v", model)
							}
							if len(result.Parts) != 2 || result.Parts[0].FunctionResponse == nil || result.Parts[0].FunctionResponse.ID != "b" || result.Parts[1].FunctionResponse == nil || result.Parts[1].FunctionResponse.ID != "a" || result.Parts[1].FunctionResponse.Name != "lookup" {
								t.Errorf("result association lost: %+v", result)
							}
						}
					}
					if streaming {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprintf(w, "data:%s\n\n", response)
					} else {
						_, _ = io.WriteString(w, response)
					}
				}))
				t.Cleanup(upstream.Close)
				respect := false
				model := config.ModelConfig{Provider: "opencode-zen", ModelID: "gemini-2.5-flash", MaxTokens: 100, Vision: config.BoolPtr(true)}
				srv, err := NewServer(config.NewAtomicConfig(&config.Config{
					ActiveSite: "opencode-zen", OpenCodeZen: config.OpenCodeZenConfig{GeminiBaseURL: upstream.URL + "/models", APIKey: "synthetic"},
					Models: map[string]config.ModelConfig{"default": model, "vision": model}, RespectRequestedModel: &respect,
					Fallbacks: map[string][]config.ModelConfig{"vision": {model}},
					Storage:   &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
				}, ""), nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = srv.Close() })
				var request map[string]any
				initial := `{"model":"m","max_tokens":100,"system":"instructions","messages":[{"role":"user","content":[{"type":"text","text":"lookup"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"YQ=="}}]}],"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"q":{"type":"integer"}}}}]}`
				if path == "/v1/responses" {
					initial = `{"model":"m","instructions":"instructions","input":[{"role":"user","content":[{"type":"input_text","text":"lookup"},{"type":"input_image","image_url":"data:image/png;base64,YQ=="}]}],"tools":[{"type":"function","name":"lookup","strict":false,"parameters":{"type":"object","properties":{"q":{"type":"integer"}}}}]}`
				}
				if err := json.Unmarshal([]byte(initial), &request); err != nil {
					t.Fatal(err)
				}
				request["stream"] = streaming
				for round := 0; round < 2; round++ {
					payload, err := json.Marshal(request)
					if err != nil {
						t.Fatal(err)
					}
					w := httptest.NewRecorder()
					srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(payload))))
					if w.Code != 200 {
						t.Fatalf("round %d HTTP %d: %s", round, w.Code, w.Body.String())
					}
					if round == 1 {
						if !strings.Contains(w.Body.String(), "done") {
							t.Fatalf("second round never completed: %s", w.Body.String())
						}
						continue
					}
					if path == "/v1/messages" {
						output := geminiTestMessageOutput(t, w.Body.Bytes(), streaming)
						if output.StopReason != "tool_use" || len(output.Content) != 3 {
							t.Fatalf("first response=%+v", output)
						}
						messages := request["messages"].([]any)
						messages = append(messages, map[string]any{"role": "assistant", "content": output.Content}, map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": output.Content[2].ID, "content": "two"}, {"type": "tool_result", "tool_use_id": output.Content[1].ID, "content": "one"}}})
						request["messages"] = messages
					} else {
						output := geminiTestResponsesOutput(t, w.Body.Bytes(), streaming)
						if len(output) != 3 {
							t.Fatalf("Responses output=%+v", output)
						}
						input := request["input"].([]any)
						for _, item := range output {
							input = append(input, item)
						}
						input = append(input, map[string]any{"type": "function_call_output", "call_id": output[2]["call_id"], "output": "two"}, map[string]any{"type": "function_call_output", "call_id": output[1]["call_id"], "output": "one"})
						request["input"] = input
					}
				}
				if calls != 2 {
					t.Fatalf("upstream attempts=%d want=2", calls)
				}
			})
		}
	}
}

func geminiTestMessageOutput(t *testing.T, body []byte, streaming bool) types.MessageResponse {
	t.Helper()
	var out types.MessageResponse
	if !streaming {
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event types.MessageEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		switch event.Type {
		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				event.ContentBlock.Input = nil
			}
			out.Content = append(out.Content, *event.ContentBlock)
		case "content_block_delta":
			block := &out.Content[*event.Index]
			block.Text += event.Delta.Text
			block.Thinking += event.Delta.Thinking
			block.Input = append(block.Input, event.Delta.PartialJSON...)
		case "message_delta":
			out.StopReason = event.Delta.StopReason
		case "error":
			t.Fatalf("stream error: %+v", event.Error)
		}
	}
	return out
}

func geminiTestResponsesOutput(t *testing.T, body []byte, streaming bool) []map[string]any {
	t.Helper()
	if streaming {
		var completed json.RawMessage
		for _, line := range strings.Split(string(body), "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event struct {
				Type     string
				Response json.RawMessage
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatal(err)
			}
			if event.Type == "response.completed" {
				completed = event.Response
			}
		}
		if completed == nil {
			t.Fatalf("no completed response: %s", body)
		}
		body = completed
	}
	var out struct {
		Status string
		Output []map[string]any
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "completed" {
		t.Fatalf("status=%s", out.Status)
	}
	return out.Output
}
