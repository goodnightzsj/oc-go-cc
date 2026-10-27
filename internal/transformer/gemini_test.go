package transformer

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func mustGeminiRequest(t *testing.T, req *types.MessageRequest, model config.ModelConfig) *types.GeminiRequest {
	t.Helper()
	wire, err := AnthropicToGemini(req, model)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func TestGeminiMapsConversationRolesAtTheWireBoundary(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{
		{Role: "user", Content: json.RawMessage(`"question"`)},
		{Role: "assistant", Content: json.RawMessage(`"answer"`)},
		{Role: "user", Content: json.RawMessage(`"next"`)},
	}}
	wire := mustGeminiRequest(t, req, config.ModelConfig{ModelID: "gemini-3.8-flash"})
	if len(wire.Contents) != 3 || wire.Contents[0].Role != "user" || wire.Contents[1].Role != "model" || wire.Contents[2].Role != "user" {
		t.Fatalf("Gemini conversation roles = %+v", wire.Contents)
	}
	if req.Messages[1].Role != "assistant" {
		t.Fatal("source request mutated")
	}
}

func TestGeminiModelResponseProducesAssistantContent(t *testing.T) {
	var response types.GeminiResponse
	if err := json.Unmarshal([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"plan","thought":true},{"text":"42"}]},"finishReason":"STOP"}]}`), &response); err != nil {
		t.Fatal(err)
	}
	normalized, err := GeminiToNormalized(&response, "gemini-3.8-flash")
	if err != nil {
		t.Fatal(err)
	}
	got := core.DenormalizeResponse(normalized)
	if got.Role != "assistant" || len(got.Content) != 2 || got.Content[0].Type != "thinking" || got.Content[0].Thinking != "plan" || got.Content[1].Text != "42" || got.StopReason != "end_turn" {
		t.Fatalf("Gemini response = %+v", got)
	}
}

func TestGeminiLevelsMatchTheTargetModel(t *testing.T) {
	for _, tc := range []struct{ model, effort, want string }{
		{"gemini-3.8-flash", "minimal", "low"},
		{"gemini-4-argon", "none", "low"},
		{"gemini-4-argon", "minimal", "low"},
		{"gemini-3.6-flash", "minimal", "minimal"},
		{"gemini-3.1-flash-lite-image", "low", "minimal"},
		{"gemini-3.1-flash-lite-image", "medium", "high"},
	} {
		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
			req := &types.MessageRequest{OutputConfig: json.RawMessage(`{"effort":"` + tc.effort + `"}`)}
			got := mustGeminiRequest(t, req, config.ModelConfig{ModelID: tc.model}).GenerationConfig.ThinkingConfig
			if got == nil || got.ThinkingLevel == nil || *got.ThinkingLevel != tc.want || got.ThinkingBudget != nil {
				t.Fatalf("thinking config = %+v, want level %q only", got, tc.want)
			}
		})
	}
}

func TestGeminiPreservesThinkingModes(t *testing.T) {
	for _, tc := range []struct{ name, model, effort, thinking, want string }{
		{"auto level", "gemini-3.8-flash", "auto", "", `null`},
		{"adaptive level", "gemini-3.8-flash", "adaptive", "", `null`},
		{"disabled wins", "gemini-2.5-flash", "high", `{"type":"disabled"}`, `{"thinkingBudget":0}`},
		{"disabled level", "gemini-3.8-flash", "high", `{"type":"disabled"}`, `{"thinkingLevel":"low","includeThoughts":true}`},
		{"zero", "gemini-2.5-flash", "", `{"type":"enabled","budget_tokens":0}`, `{"thinkingBudget":0}`},
		{"dynamic", "gemini-2.5-flash-lite", "", `{"type":"enabled","budget_tokens":-1}`, `{"thinkingBudget":-1}`},
		{"adaptive budget", "gemini-2.5-flash-lite", "", `{"type":"adaptive"}`, `{"thinkingBudget":-1}`},
		{"auto budget", "gemini-2.5-flash-lite", "auto", "", `{"thinkingBudget":-1}`},
		{"unspecified", "gemini-2.5-flash-lite", "", "", `null`},
		{"pro cannot disable", "gemini-2.5-pro", "", `{"type":"disabled"}`, `{"thinkingBudget":128}`},
		{"none budget", "gemini-2.5-flash", "none", "", `{"thinkingBudget":0}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &types.MessageRequest{Thinking: json.RawMessage(tc.thinking)}
			if tc.effort != "" {
				req.OutputConfig = json.RawMessage(`{"effort":"` + tc.effort + `"}`)
			}
			got, err := json.Marshal(mustGeminiRequest(t, req, config.ModelConfig{ModelID: tc.model}).GenerationConfig.ThinkingConfig)
			if err != nil || string(got) != tc.want {
				t.Fatalf("thinking config = %s (%v), want %s", got, err, tc.want)
			}
		})
	}
}

func TestGeminiResponseSeparatesThoughtsAndPreservesPartOrder(t *testing.T) {
	var response types.GeminiResponse
	if err := json.Unmarshal([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"plan","thought":true},{"text":"answer"},{"thought":true},{"text":"check","thought":true},{"text":"done"}]},"finishReason":"STOP"}]}`), &response); err != nil {
		t.Fatal(err)
	}
	normalized, err := GeminiToNormalized(&response, "gemini-3.8-flash")
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for _, message := range normalized.Messages {
		if message.Thinking != "" {
			parts = append(parts, "thinking:"+message.Thinking)
		}
		if message.Content != "" {
			parts = append(parts, "text:"+message.Content)
		}
	}
	want := []string{"thinking:plan", "text:answer", "thinking:check", "text:done"}
	if !reflect.DeepEqual(parts, want) {
		t.Fatalf("parts = %v, want %v", parts, want)
	}
}

func TestGeminiStreamSeparatesThoughtsAndClosesEveryBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	w := newMockResponseWriter()
	body := sseLines(
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"A","thought":true}]}}]}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"B","thought":true},{"text":"42"},{"text":"check","thought":true}]}}]}`,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"!"}]},"finishReason":"STOP"}]}`,
	)
	if err := NewStreamHandler().ProxyGeminiStream(w, body, "gemini-3.8-flash", ctx, 0, cancel); err != nil {
		t.Fatal(err)
	}
	var parts []string
	open, stops := -1, 0
	for _, event := range parseSSEEvents(t, w.buf.String()) {
		switch event.Type {
		case "content_block_start":
			if open != -1 || event.Index == nil || *event.Index != len(parts) || event.ContentBlock == nil {
				t.Fatalf("invalid block start: %+v, open=%d", event, open)
			}
			open = *event.Index
			parts = append(parts, event.ContentBlock.Type+":")
		case "content_block_delta":
			if open < 0 || event.Index == nil || *event.Index != open || event.Delta == nil {
				t.Fatalf("delta outside its block: %+v", event)
			}
			switch event.Delta.Type {
			case "thinking_delta":
				parts[open] += event.Delta.Thinking
			case "text_delta":
				parts[open] += event.Delta.Text
			}
		case "content_block_stop":
			if open < 0 || event.Index == nil || *event.Index != open {
				t.Fatalf("invalid block stop: %+v", event)
			}
			open = -1
		case "message_stop":
			stops++
		}
	}
	want := []string{"thinking:AB", "text:42", "thinking:check", "text:!"}
	if !reflect.DeepEqual(parts, want) || open != -1 || stops != 1 {
		t.Fatalf("parts=%v open=%d message stops=%d, want %v", parts, open, stops, want)
	}
}
