package transformer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func TestGeminiFinalPayloadPreservesSystemImagesAndControls(t *testing.T) {
	var req types.MessageRequest
	if err := json.Unmarshal([]byte(`{"system":[{"type":"text","text":"system first"}],"temperature":0,"top_p":0.7,"stop_sequences":["END"],"tool_choice":{"type":"tool","name":"lookup"},"output_config":{"format":{"type":"json_schema","schema":{"type":"object","properties":{},"additionalProperties":false}}},"messages":[{"role":"user","content":[{"type":"text","text":"before"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGk="}},{"type":"text","text":"after"}]}]}`), &req); err != nil {
		t.Fatal(err)
	}
	wire := mustGeminiRequest(t, &req, config.ModelConfig{ModelID: "gemini-2.5-flash"})
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		System     types.GeminiContent        `json:"systemInstruction"`
		Contents   []types.GeminiContent      `json:"contents"`
		Generation map[string]json.RawMessage `json:"generationConfig"`
		Tool       types.GeminiToolConfig     `json:"toolConfig"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.System.Parts) != 1 || body.System.Parts[0].Text != "system first" || len(body.Contents) != 1 {
		t.Fatalf("system downgraded: %s", raw)
	}
	parts := body.Contents[0].Parts
	if len(parts) != 3 || parts[0].Text != "before" || parts[1].InlineData == nil || parts[1].InlineData.Data != "aGk=" || parts[1].InlineData.MIMEType != "image/png" || parts[2].Text != "after" {
		t.Fatalf("image order lost: %s", raw)
	}
	if string(body.Generation["temperature"]) != "0" || string(body.Generation["topP"]) != "0.7" || string(body.Generation["stopSequences"]) != `["END"]` || string(body.Generation["responseMimeType"]) != `"application/json"` || len(body.Generation["responseJsonSchema"]) == 0 || body.Tool.FunctionCallingConfig.Mode != "ANY" || body.Tool.FunctionCallingConfig.AllowedFunctionNames[0] != "lookup" {
		t.Fatalf("controls lost: %s", raw)
	}
}

func TestGeminiToolCallsRoundTripWithReorderedSameNameResults(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "stream"}[streaming], func(t *testing.T) {
			const source = `{"candidates":[{"content":{"role":"model","parts":[{"text":"before"},{"functionCall":{"id":"a","name":"lookup","args":{"q":1}}},{"functionCall":{"id":"b","name":"lookup","args":{"q":2}}}]},"finishReason":"STOP"}]}`
			var response types.MessageResponse
			if !streaming {
				var upstream types.GeminiResponse
				if err := json.Unmarshal([]byte(source), &upstream); err != nil {
					t.Fatal(err)
				}
				nr, err := GeminiToNormalized(&upstream, "gemini-2.5-flash")
				if err != nil {
					t.Fatal(err)
				}
				response = *core.DenormalizeResponse(nr)
			} else {
				w := newMockResponseWriter()
				if err := NewStreamHandler().ProxyGeminiStream(w, sseLines(source), "gemini-2.5-flash", t.Context(), 0, func() {}); err != nil {
					t.Fatal(err)
				}
				for _, event := range parseSSEEvents(t, w.buf.String()) {
					switch event.Type {
					case "content_block_start":
						if event.ContentBlock.Type == "tool_use" {
							// The start envelope's {} is not an argument fragment.
							event.ContentBlock.Input = nil
						}
						response.Content = append(response.Content, *event.ContentBlock)
					case "content_block_delta":
						block := &response.Content[*event.Index]
						if event.Delta.Type == "input_json_delta" {
							block.Input = append(block.Input, event.Delta.PartialJSON...)
						} else if event.Delta.Type == "text_delta" {
							block.Text += event.Delta.Text
						}
					case "message_delta":
						response.StopReason = event.Delta.StopReason
					}
				}
			}
			if response.StopReason != "tool_use" || len(response.Content) != 3 || response.Content[1].ID != "a" || response.Content[2].ID != "b" {
				t.Fatalf("tool response lost: %+v", response)
			}
			history, err := json.Marshal(response.Content)
			if err != nil {
				t.Fatal(err)
			}
			req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"lookup"`)}, {Role: "assistant", Content: history}, {Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"b","content":"two"},{"type":"tool_result","tool_use_id":"a","content":"one","is_error":true}]`)}}}
			raw, err := json.Marshal(mustGeminiRequest(t, req, config.ModelConfig{ModelID: "gemini-2.5-flash"}))
			if err != nil {
				t.Fatal(err)
			}
			var wire types.GeminiRequest
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			parts := wire.Contents[2].Parts
			if wire.Contents[1].Role != "model" || len(parts) != 2 || parts[0].FunctionResponse.ID != "b" || parts[1].FunctionResponse.ID != "a" || parts[1].FunctionResponse.Name != "lookup" || string(parts[1].FunctionResponse.Response) != `{"error":"one"}` {
				t.Fatalf("second round lost correlation: %s", raw)
			}
		})
	}
}

func TestGeminiRejectsUnrepresentableHistory(t *testing.T) {
	for _, content := range []string{
		`[{"type":"tool_result","tool_use_id":"missing","content":"lost"}]`,
		`[{"type":"tool_use","id":"a","name":"lookup","input":[]}]`,
		`[{"type":"thinking","thinking":"text","signature":"real-anthropic-signature"}]`,
		`[{"type":"document","source":{}}]`,
	} {
		req := &types.MessageRequest{Messages: []types.Message{{Role: "assistant", Content: json.RawMessage(content)}}}
		if _, err := AnthropicToGemini(req, config.ModelConfig{ModelID: "gemini-2.5-flash"}); err == nil {
			t.Errorf("silently accepted %s", content)
		}
	}
}

func TestCrossProtocolOutputControls(t *testing.T) {
	var req types.MessageRequest
	if err := json.Unmarshal([]byte(`{"temperature":0,"stop_sequences":["END"],"output_config":{"format":{"type":"json_schema","schema":{"type":"object","properties":{},"additionalProperties":false}}}}`), &req); err != nil {
		t.Fatal(err)
	}
	chat, err := AnthropicToChatCompletion(&req, config.ModelConfig{ModelID: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(chat)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"temperature":0`) || !strings.Contains(string(raw), `"stop":["END"]`) {
		t.Fatalf("Chat controls lost: %s", raw)
	}
	if _, err := AnthropicToResponses(&req, config.ModelConfig{ModelID: "synthetic"}); err == nil {
		t.Fatal("Responses silently dropped stop")
	}
	req.StopSequences = nil
	responses, err := AnthropicToResponses(&req, config.ModelConfig{ModelID: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if string(responses.Text) != `{"format":{"name":"response","schema":{"type":"object","properties":{},"additionalProperties":false},"strict":true,"type":"json_schema"}}` {
		t.Fatalf("Responses format lost: %s", responses.Text)
	}
}
