package transformer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

// TestTransformRequestRoundTripReasoning verifies that a DeepSeek response with
// reasoning_content survives the full round-trip (OpenAI response → Anthropic
// response → Anthropic request → OpenAI request) so that on the next turn
// DeepSeed receives the reasoning_content it expects.
func TestTransformRequestRoundTripReasoning(t *testing.T) {
	// Step 1: Simulate a DeepSeek response with reasoning_content.
	deepSeekReasoning := "Let me think step by step"
	openaiResp := &types.ChatCompletionResponse{
		ID:     "resp_123",
		Object: "chat.completion",
		Model:  "deepseek-v4-flash",
		Choices: []types.Choice{{
			Index: 0,
			Message: types.ChatMessage{
				Role:             "assistant",
				Content:          contentText("The answer is 42"),
				ReasoningContent: &deepSeekReasoning,
			},
			FinishReason: "stop",
		}},
		Usage: types.UsageInfo{
			PromptTokens:     10,
			CompletionTokens: 20,
		},
	}

	// Step 2: Transform to Anthropic format (what Claude Code receives).
	rt := NewResponseTransformer()
	anthropicResp, err := rt.TransformResponse(openaiResp, "deepseek-v4-flash")
	if err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}

	// Verify Anthropic response has a thinking block.
	if len(anthropicResp.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(anthropicResp.Content))
	}
	if anthropicResp.Content[0].Type != "thinking" {
		t.Fatalf("expected first block to be thinking, got %s", anthropicResp.Content[0].Type)
	}
	if anthropicResp.Content[0].Thinking != deepSeekReasoning {
		t.Fatalf("thinking text = %q, want %q", anthropicResp.Content[0].Thinking, deepSeekReasoning)
	}

	// Step 3: Simulate Claude Code sending the conversation back on the next turn.
	// It includes the previous assistant message with the thinking block.
	anthropicReq := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"What is the answer?"`)},
			{
				Role:    "assistant",
				Content: mustJSONBytes(t, anthropicResp.Content),
			},
			{Role: "user", Content: json.RawMessage(`"Explain why?"`)},
		},
	}

	// Step 4: Transform back to OpenAI request.
	qt := NewRequestTransformer()
	openaiReq, err := qt.TransformRequest(anthropicReq, config.ModelConfig{ModelID: "deepseek-v4-flash"})
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}

	// Find the assistant message.
	var assistantMsg *types.ChatMessage
	for i := range openaiReq.Messages {
		if openaiReq.Messages[i].Role == "assistant" {
			assistantMsg = &openaiReq.Messages[i]
			break
		}
	}
	if assistantMsg == nil {
		t.Fatal("assistant message not found in transformed request")
		return
	}

	// Step 5: Verify reasoning_content is preserved.
	if assistantMsg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil after round-trip")
	}
	if got, want := *assistantMsg.ReasoningContent, deepSeekReasoning; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}

	// Also verify the JSON serialization includes the field.
	body, err := json.Marshal(openaiReq)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	if !bytes.Contains(body, []byte(`"reasoning_content"`)) {
		t.Fatalf("serialized request missing reasoning_content field: %s", body)
	}
}

func TestTransformRequestPreservesThinkingAsReasoningContent(t *testing.T) {
	transformer := NewRequestTransformer()
	stream := true

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Stream:    &stream,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Need to look up the weather first","signature":"sig_123"},
					{"type":"tool_use","id":"toolu_123","name":"get_weather","input":{"city":"Kigali"}}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	msg := openaiReq.Messages[0]
	if got, want := msg.Role, "assistant"; got != want {
		t.Fatalf("Role = %q, want %q", got, want)
	}
	if msg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil")
	}
	if got, want := *msg.ReasoningContent, "Need to look up the weather first"; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
	if got, want := len(msg.ToolCalls), 1; got != want {
		t.Fatalf("len(ToolCalls) = %d, want %d", got, want)
	}
	if got, want := msg.ToolCalls[0].ID, "toolu_123"; got != want {
		t.Fatalf("ToolCalls[0].ID = %q, want %q", got, want)
	}
	if got, want := msg.ToolCalls[0].Function.Name, "get_weather"; got != want {
		t.Fatalf("ToolCalls[0].Function.Name = %q, want %q", got, want)
	}
	if got, want := msg.ToolCalls[0].Function.Arguments, `{"city":"Kigali"}`; got != want {
		t.Fatalf("ToolCalls[0].Function.Arguments = %q, want %q", got, want)
	}
}

func TestTransformRequestIncludesStreamUsageOptions(t *testing.T) {
	transformer := NewRequestTransformer()
	stream := true

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Stream:    &stream,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.StreamOptions == nil {
		t.Fatal("StreamOptions = nil, want include_usage enabled")
	}
	if !openaiReq.StreamOptions.IncludeUsage {
		t.Fatal("StreamOptions.IncludeUsage = false, want true")
	}
}

func TestTransformRequestOmitsStreamUsageOptionsWhenStreamingDisabled(t *testing.T) {
	transformer := NewRequestTransformer()
	stream := false

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Stream:    &stream,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.StreamOptions != nil {
		t.Fatalf("StreamOptions = %v, want nil when streaming is disabled", openaiReq.StreamOptions)
	}
}

func TestTransformRequestIncludesEmptyReasoningContentForToolCalls(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"tool_use","id":"toolu_456","name":"search_docs","input":{"query":"figma api"}}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	msg := openaiReq.Messages[0]
	if msg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil placeholder")
	}
	if got, want := *msg.ReasoningContent, " "; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
}

func TestTransformRequestSerializesAssistantToolCallContent(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"tool_use","id":"toolu_456","name":"search_docs","input":{"query":"figma api"}}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	body, err := json.Marshal(openaiReq)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var payload struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := payload.Messages[0]["content"]; !ok {
		t.Fatalf("serialized assistant tool-call message omitted content: %s", body)
	}
	if got, want := string(payload.Messages[0]["content"]), `""`; got != want {
		t.Fatalf("serialized content = %s, want %s", got, want)
	}
}

func TestTransformRequestAppliesReasoningEffortAndThinking(t *testing.T) {
	transformer := NewRequestTransformer()

	// When the conversation history already contains thinking blocks,
	// reasoning_effort and thinking should be applied.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"solve this carefully"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Let me think..."},
					{"type":"text","text":"The answer is 42"}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
		Thinking:        json.RawMessage(`{"type":"enabled"}`),
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort == nil {
		t.Fatal("ReasoningEffort = nil, want max")
	}
	if got, want := *openaiReq.ReasoningEffort, "max"; got != want {
		t.Fatalf("ReasoningEffort = %q, want %q", got, want)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"enabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

// Claude Code's generate_session_title sends output_config.format, so the Chat
// Completions adapter must forward it as response_format rather than 502.
func TestTransformRequestMapsStructuredOutputFormat(t *testing.T) {
	transformer := NewRequestTransformer()
	schema := `{"type":"object","properties":{"title":{"type":"string"}},"required":["title"],"additionalProperties":false}`

	req := &types.MessageRequest{
		Model:        "claude-test",
		MaxTokens:    64,
		OutputConfig: json.RawMessage(`{"format":{"type":"json_schema","schema":` + schema + `}}`),
		Messages:     []types.Message{{Role: "user", Content: json.RawMessage(`"name this session"`)}},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	var got struct {
		Type       string `json:"type"`
		JSONSchema struct {
			Name   string          `json:"name"`
			Schema json.RawMessage `json:"schema"`
			Strict bool            `json:"strict"`
		} `json:"json_schema"`
	}
	if err := json.Unmarshal(openaiReq.ResponseFormat, &got); err != nil {
		t.Fatalf("response_format = %s, want a JSON object: %v", openaiReq.ResponseFormat, err)
	}
	if got.Type != "json_schema" || got.JSONSchema.Name != "response" || !got.JSONSchema.Strict {
		t.Fatalf("response_format = %s, want a strict json_schema named response", openaiReq.ResponseFormat)
	}
	if string(got.JSONSchema.Schema) != schema {
		t.Fatalf("schema = %s, want %s", got.JSONSchema.Schema, schema)
	}
}

func TestTransformRequestRejectsUnknownStructuredOutputFormat(t *testing.T) {
	transformer := NewRequestTransformer()
	req := &types.MessageRequest{
		Model:        "claude-test",
		MaxTokens:    64,
		OutputConfig: json.RawMessage(`{"format":{"type":"text"}}`),
		Messages:     []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}

	if _, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"}); err == nil {
		t.Fatal("TransformRequest() error = nil, want a non-json_schema format rejected")
	}
}

func TestTransformRequestDeepSeekHistoryGuardOverridesExplicitThinking(t *testing.T) {
	transformer := NewRequestTransformer()

	// DeepSeek rejects thinking mode when historical assistant messages lack
	// reasoning_content, so the safety guard must win over explicit config.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"solve this carefully"`)},
			{Role: "assistant", Content: json.RawMessage(`"The answer is 42"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
		Thinking:        json.RawMessage(`{"type":"enabled"}`),
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort = %v, want nil (DeepSeek history guard)", *openaiReq.ReasoningEffort)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

func TestTransformRequestFirstTurnEnablesThinkingWithReasoningEffort(t *testing.T) {
	transformer := NewRequestTransformer()

	// First turn (no assistant messages in history), only reasoning_effort
	// set in config → thinking should be enabled so DeepSeek can produce
	// reasoning content from the very first response.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"solve this carefully"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort == nil {
		t.Fatal("ReasoningEffort = nil, want max on first turn")
	}
	if got, want := *openaiReq.ReasoningEffort, "max"; got != want {
		t.Fatalf("ReasoningEffort = %q, want %q", got, want)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"enabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

func TestTransformRequestRequestDisabledThinkingSkipsReasoningEffort(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Thinking:  json.RawMessage(`{"type":"disabled"}`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"solve this carefully"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort = %v, want nil when request disables thinking", *openaiReq.ReasoningEffort)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

func TestTransformRequestThinkingDecisionMatrix(t *testing.T) {
	transformer := NewRequestTransformer()

	userOnly := []types.Message{
		{Role: "user", Content: json.RawMessage(`"solve this carefully"`)},
	}
	plainAssistantHistory := []types.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`"hi"`)},
		{Role: "user", Content: json.RawMessage(`"explain"`)},
	}
	thinkingHistory := []types.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{
			Role: "assistant",
			Content: json.RawMessage(`[
				{"type":"thinking","thinking":"Let me think..."},
				{"type":"text","text":"The answer is 42"}
			]`),
		},
		{Role: "user", Content: json.RawMessage(`"explain"`)},
	}

	tests := []struct {
		name       string
		messages   []types.Message
		thinking   json.RawMessage
		model      config.ModelConfig
		wantThink  string
		wantEffort *string
	}{
		{
			// DeepSeek takes thinking.budget_tokens, so the budget is forwarded
			// as-is and no derived effort is added: two controls for one
			// decision, and the level is a lossy restatement of the budget.
			name:      "deepseek request thinking forwards the budget without a derived effort",
			messages:  userOnly,
			thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":4096}`),
			model:     config.ModelConfig{ModelID: "deepseek-v4-pro"},
			wantThink: `{"type":"enabled","budget_tokens":4096}`,
		},
		{
			name:      "deepseek plain assistant history guard beats request thinking",
			messages:  plainAssistantHistory,
			thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":4096}`),
			model:     config.ModelConfig{ModelID: "deepseek-v4-pro"},
			wantThink: `{"type":"disabled"}`,
		},
		{
			name:      "deepseek request disabled beats thinking history and effort",
			messages:  thinkingHistory,
			thinking:  json.RawMessage(`{"type":"disabled"}`),
			model:     config.ModelConfig{ModelID: "deepseek-v4-pro", ReasoningEffort: "max"},
			wantThink: `{"type":"disabled"}`,
		},
		{
			name:      "openai reasoning model maps request budget without thinking field",
			messages:  userOnly,
			thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":2048}`),
			model:     config.ModelConfig{ModelID: "o3-mini"},
			wantThink: "",
			wantEffort: func() *string {
				s := "low"
				return &s
			}(),
		},
		{
			name:      "openai reasoning model uses explicit effort without thinking field",
			messages:  userOnly,
			model:     config.ModelConfig{ModelID: "o3-mini", ReasoningEffort: "max"},
			wantThink: "",
			wantEffort: func() *string {
				s := "max"
				return &s
			}(),
		},
		{
			name:      "standard model ignores request thinking",
			messages:  userOnly,
			thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":2048}`),
			model:     config.ModelConfig{ModelID: "qwen3.6-plus"},
			wantThink: "",
		},
		{
			name:      "kimi ignores request and history thinking without explicit capability",
			messages:  thinkingHistory,
			thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":4096}`),
			model:     config.ModelConfig{ModelID: "kimi-k2.6"},
			wantThink: "",
		},
		{
			name:      "request disabled overrides explicit model thinking",
			messages:  userOnly,
			thinking:  json.RawMessage(`{"type":"disabled"}`),
			model:     config.ModelConfig{ModelID: "deepseek-v4-pro", Thinking: json.RawMessage(`{"type":"enabled"}`), ReasoningEffort: "max"},
			wantThink: `{"type":"disabled"}`,
		},
		{
			name:      "model disabled thinking skips explicit effort",
			messages:  userOnly,
			model:     config.ModelConfig{ModelID: "deepseek-v4-pro", Thinking: json.RawMessage(`{"type":"disabled"}`), ReasoningEffort: "max"},
			wantThink: `{"type":"disabled"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &types.MessageRequest{
				Model:     "claude-test",
				MaxTokens: 256,
				Thinking:  tt.thinking,
				Messages:  tt.messages,
			}

			openaiReq, err := transformer.TransformRequest(req, tt.model)
			if err != nil {
				t.Fatalf("TransformRequest() error = %v", err)
			}

			if got := string(openaiReq.Thinking); got != tt.wantThink {
				t.Fatalf("Thinking = %s, want %s", got, tt.wantThink)
			}
			if tt.wantEffort == nil {
				if openaiReq.ReasoningEffort != nil {
					t.Fatalf("ReasoningEffort = %v, want nil", *openaiReq.ReasoningEffort)
				}
				return
			}
			if openaiReq.ReasoningEffort == nil {
				t.Fatalf("ReasoningEffort = nil, want %s", *tt.wantEffort)
			}
			if got, want := *openaiReq.ReasoningEffort, *tt.wantEffort; got != want {
				t.Fatalf("ReasoningEffort = %s, want %s", got, want)
			}
		})
	}
}

func TestTransformRequestFirstTurnReasoningEffortDefaultsToHigh(t *testing.T) {
	transformer := NewRequestTransformer()

	// First turn with thinking in history (from previous response round-trip).
	// No explicit ReasoningEffort → defaults to "high".
	// This test also covers the no-explicit-thinking-config path.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Let me think..."},
					{"type":"text","text":"The answer is 42"}
				]`),
			},
			{Role: "user", Content: json.RawMessage(`"explain"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-pro",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort == nil {
		t.Fatal("ReasoningEffort = nil, want default high")
	}
	if got, want := *openaiReq.ReasoningEffort, "high"; got != want {
		t.Fatalf("ReasoningEffort = %q, want %q", got, want)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"enabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

func TestTransformRequestSafetyGuardWithReasoningEffortOnly(t *testing.T) {
	transformer := NewRequestTransformer()

	// Only ReasoningEffort set (no explicit Thinking). History has an
	// assistant message without thinking blocks + it's a DeepSeek model
	// → safety guard fires, thinking is disabled to prevent the 400
	// "reasoning_content must be passed back" error.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "assistant", Content: json.RawMessage(`"hi"`)},
			{Role: "user", Content: json.RawMessage(`"explain"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort = %v, want nil (safety guard)", *openaiReq.ReasoningEffort)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s (safety guard)", got, want)
	}
}

func TestTransformRequestNoThinkingConfigNoHistory(t *testing.T) {
	transformer := NewRequestTransformer()

	// No Thinking, no ReasoningEffort, no thinking history → nothing set.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-pro",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort = %v, want nil", *openaiReq.ReasoningEffort)
	}
	if openaiReq.Thinking != nil {
		t.Fatalf("Thinking = %s, want nil", string(openaiReq.Thinking))
	}
}

func TestTransformRequestPreservesSystemCacheControl(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		System: json.RawMessage(`[
			{"type":"text","text":"You are helpful","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	systemMsg := openaiReq.Messages[0]
	if got, want := systemMsg.Role, "system"; got != want {
		t.Fatalf("Messages[0].Role = %q, want %q", got, want)
	}
	if got, want := systemMsg.ContentText(), "You are helpful"; got != want {
		t.Fatalf("Messages[0].Content = %q, want %q", got, want)
	}
	if systemMsg.CacheControl == nil {
		t.Fatal("Messages[0].CacheControl = nil, want non-nil")
	}
	if got, want := systemMsg.CacheControl.Type, "ephemeral"; got != want {
		t.Fatalf("Messages[0].CacheControl.Type = %q, want %q", got, want)
	}
}

func TestTransformRequestSkipsCacheControlForKimiSystem(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		System: json.RawMessage(`[
			{"type":"text","text":"system prompt","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	systemMsg := openaiReq.Messages[0]
	if got, want := systemMsg.Role, "system"; got != want {
		t.Fatalf("Messages[0].Role = %q, want %q", got, want)
	}
	if got, want := systemMsg.ContentText(), "system prompt"; got != want {
		t.Fatalf("Messages[0].Content = %q, want %q", got, want)
	}
	if systemMsg.CacheControl != nil {
		t.Fatalf("Kimi system message CacheControl = %v, want nil (guard should prevent assignment)", systemMsg.CacheControl)
	}
}

func TestTransformRequestStripsCacheControlForNonKimiNonDeepSeek(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		System: json.RawMessage(`[
			{"type":"text","text":"system prompt","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	// Use a non-Kimi, non-DeepSeek model (e.g. GLM) — cache_control should be
	// set by transformMessages then stripped by stripCacheControl().
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "glm-5"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	systemMsg := openaiReq.Messages[0]
	if got, want := systemMsg.Role, "system"; got != want {
		t.Fatalf("Messages[0].Role = %q, want %q", got, want)
	}
	if systemMsg.CacheControl != nil {
		t.Fatalf("Non-Kimi/non-DeepSeek system message CacheControl = %v, want nil", systemMsg.CacheControl)
	}
}

func TestTransformRequestStripsCacheControlForNonDeepSeek(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		System: json.RawMessage(`[
			{"type":"text","text":"You are helpful","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	systemMsg := openaiReq.Messages[0]
	if systemMsg.CacheControl != nil {
		t.Fatalf("Messages[0].CacheControl = %v, want nil for non-DeepSeek model", systemMsg.CacheControl)
	}
}

func TestTransformRequestOmitsCacheControlWhenAbsent(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		System:    json.RawMessage(`"You are helpful"`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	systemMsg := openaiReq.Messages[0]
	if got, want := systemMsg.Role, "system"; got != want {
		t.Fatalf("Messages[0].Role = %q, want %q", got, want)
	}
	if systemMsg.CacheControl != nil {
		t.Fatalf("Messages[0].CacheControl = %v, want nil", systemMsg.CacheControl)
	}
}

func TestTransformRequestPlacesToolResultsBeforeUserText(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"tool_use","id":"toolu_123","name":"create_file","input":{"name":"draft.fig"}}
				]`),
			},
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"tool_result","tool_use_id":"toolu_123","content":"created"},
					{"type":"text","text":"now continue"}
				]`),
			},
		},
	}

	// DeepSeek models preserve cache_control
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 3; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	if got, want := openaiReq.Messages[0].Role, "assistant"; got != want {
		t.Fatalf("Messages[0].Role = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[1].Role, "tool"; got != want {
		t.Fatalf("Messages[1].Role = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[1].ToolCallID, "toolu_123"; got != want {
		t.Fatalf("Messages[1].ToolCallID = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[2].Role, "user"; got != want {
		t.Fatalf("Messages[2].Role = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[2].ContentText(), "now continue"; got != want {
		t.Fatalf("Messages[2].Content = %q, want %q", got, want)
	}
}

func TestTransformRequestSkipsReasoningEffortWhenThinkingDisabled(t *testing.T) {
	transformer := NewRequestTransformer()

	// When thinking is explicitly disabled in model config, reasoning_effort
	// must NOT be set — DeepSeek returns 400 if both are present.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"think carefully"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Let me think..."},
					{"type":"text","text":"The answer is 42"}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
		Thinking:        json.RawMessage(`{"type":"disabled"}`),
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort = %v, want nil (stripped because thinking is disabled)", *openaiReq.ReasoningEffort)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

func TestTransformRequestOmitsPlaceholderForDeepSeek(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"tool_use","id":"toolu_456","name":"search_docs","input":{"query":"figma api"}}
				]`),
			},
		},
	}

	// DeepSeek should NOT get a placeholder when there's no thinking history
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	msg := openaiReq.Messages[1] // assistant message
	if msg.ReasoningContent != nil {
		t.Fatalf("ReasoningContent = %q, want nil (DeepSeek without thinking history should not get placeholder)", *msg.ReasoningContent)
	}
}

func TestTransformRequestDeepSeekPlaceholderWithThinkingHistory(t *testing.T) {
	transformer := NewRequestTransformer()

	// When thinking history exists, DeepSeek assistant messages with tool_calls
	// but no thinking block MUST get a placeholder reasoning_content, because
	// DeepSeek requires ALL assistant messages to have reasoning_content in
	// thinking mode.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"think about this"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Let me think..."},
					{"type":"text","text":"I considered it"}
				]`),
			},
			{Role: "user", Content: json.RawMessage(`"now use a tool"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"tool_use","id":"toolu_789","name":"search","input":{"q":"test"}}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-flash",
		ReasoningEffort: "high",
		Thinking:        json.RawMessage(`{"type":"enabled"}`),
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Find the second assistant message (tool_call only, no thinking)
	var toolCallAssistant *types.ChatMessage
	for i := range openaiReq.Messages {
		if openaiReq.Messages[i].Role == "assistant" && len(openaiReq.Messages[i].ToolCalls) > 0 {
			toolCallAssistant = &openaiReq.Messages[i]
			break
		}
	}
	if toolCallAssistant == nil {
		t.Fatal("no assistant message with tool_calls found")
		return
	}
	if toolCallAssistant.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil placeholder for DeepSeek with thinking history")
	}
	if *toolCallAssistant.ReasoningContent != " " {
		t.Fatalf("ReasoningContent = %q, want placeholder space", *toolCallAssistant.ReasoningContent)
	}
}

// Regression test for an upstream 400 observed in production:
//
//	"The reasoning_content in the thinking mode must be passed back to the API."
//
// Claude Code can produce assistant turns that are text-only (no tool_use,
// no thinking block) inside a conversation where an earlier turn DID carry
// thinking. Examples: a follow-up that the model answers in plain text, or
// a post-/compact summary message. DeepSeek in thinking mode requires
// reasoning_content on EVERY assistant message, not just tool_use ones, so
// the proxy must add the placeholder for text-only turns too.
func TestTransformRequestDeepSeekPlaceholderForTextOnlyAssistant(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"think about this"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Let me think..."},
					{"type":"text","text":"My initial answer"}
				]`),
			},
			{Role: "user", Content: json.RawMessage(`"a follow-up question"`)},
			{
				// Text-only continuation. No thinking block (Claude Code
				// commonly drops thinking on simple follow-ups), no tool_use.
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"text","text":"A short reply"}]`),
			},
			{Role: "user", Content: json.RawMessage(`"and another"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-flash",
		ReasoningEffort: "high",
		Thinking:        json.RawMessage(`{"type":"enabled"}`),
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Find the second assistant message (text-only follow-up).
	var textOnlyAssistant *types.ChatMessage
	seen := 0
	for i := range openaiReq.Messages {
		if openaiReq.Messages[i].Role != "assistant" {
			continue
		}
		seen++
		if seen == 2 {
			textOnlyAssistant = &openaiReq.Messages[i]
			break
		}
	}
	if textOnlyAssistant == nil {
		t.Fatal("expected two assistant messages in transformed request, found fewer")
		return
	}
	if len(textOnlyAssistant.ToolCalls) != 0 {
		t.Fatalf("text-only assistant message unexpectedly had tool_calls: %+v", textOnlyAssistant.ToolCalls)
	}

	// The bug this test guards against: ReasoningContent was nil on this
	// message, causing DeepSeek to 400 the entire request. After the fix,
	// it's the single-space placeholder.
	if textOnlyAssistant.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil placeholder for DeepSeek text-only assistant in thinking-mode conversation")
	}
	if got, want := *textOnlyAssistant.ReasoningContent, " "; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
}

// Regression test for the production failure that motivated this PR.
//
// User configured oc-go-cc with a bare DeepSeek model config — no
// `thinking` field, no `reasoning_effort`. They ran Claude Code with
// `effortLevel: xhigh` set globally. Workflow:
//
//	turn 1: user asks question  →  proxy forwards to deepseek-v4-flash
//	turn 1 response: succeeds, upstream ran in DeepSeek's *default*
//	                 thinking mode (DeepSeek-v4 always defaults to
//	                 thinking mode unless explicitly disabled)
//	turn 2: user follows up  →  proxy receives request, sees no thinking
//	                             blocks in history (Claude Code didn't
//	                             round-trip the reasoning back), forwards
//	                             with `openaiReq.Thinking = nil` because
//	                             neither model config nor history asked for
//	                             thinking-mode handling
//	turn 2: upstream is STILL in thinking mode (default), demands
//	        reasoning_content on the prior assistant message which the
//	        proxy didn't add → 400.
//
// Fix: when the model is DeepSeek and there's no extant thinking history,
// explicitly send `thinking: disabled` so upstream switches off thinking
// mode and stops demanding reasoning_content.
func TestTransformRequestForceDisablesThinkingForDeepSeekWithoutHistory(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "assistant", Content: json.RawMessage(`"hi"`)},
			{Role: "user", Content: json.RawMessage(`"do something"`)},
		},
	}

	// Bare DeepSeek config: no Thinking field, no ReasoningEffort.
	// Mirrors a typical user setup for the `fast` slot.
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Must explicitly disable thinking — leaving it nil lets DeepSeek's
	// default thinking-mode behavior take over and 400 on subsequent turns.
	if len(openaiReq.Thinking) == 0 {
		t.Fatal("openaiReq.Thinking is empty — must be set to {\"type\":\"disabled\"} for DeepSeek without thinking history")
	}
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("openaiReq.Thinking = %s, want %s", got, want)
	}
}

// Regression test: Claude Code emits tool_use blocks with the chain-of-
// thought attached directly via a `thinking` field, instead of as a
// separate thinking-typed block. Real shape observed:
//
//	{"type":"tool_use","id":"toolu_X","name":"...","input":{...},
//	 "thinking":"reasoning that led to the tool call"}
//
// HasThinkingBlocks must recognize this as thinking history, and
// transformAssistantMessage must extract the inline thinking string into
// reasoning_content. Without this:
//   - HasThinkingBlocks returns false → thinking mode not detected →
//     placeholder branch never fires → DeepSeek (which is in thinking mode
//     after the first reasoning response from the upstream-default mode)
//     returns 400 on the next request.
func TestTransformRequestExtractsThinkingFromToolUseBlock(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"search for X"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{
						"type":"tool_use",
						"id":"toolu_thinking_inline",
						"name":"search",
						"input":{"q":"X"},
						"thinking":"I need to search the docs first"
					}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// (1) HasThinkingBlocks must detect the inline thinking on tool_use.
	if !HasThinkingBlocks(req.Messages) {
		t.Fatal("HasThinkingBlocks = false, want true (tool_use block has inline thinking)")
	}

	// (2) The transformed assistant message must carry the thinking content
	//     as reasoning_content so DeepSeek's validator is satisfied.
	var assistantMsg *types.ChatMessage
	for i := range openaiReq.Messages {
		if openaiReq.Messages[i].Role == "assistant" {
			assistantMsg = &openaiReq.Messages[i]
			break
		}
	}
	if assistantMsg == nil {
		t.Fatal("no assistant message in transformed request")
		return
	}
	if assistantMsg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil (thinking on tool_use must round-trip)")
	}
	if got, want := *assistantMsg.ReasoningContent, "I need to search the docs first"; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
	// (3) tool_calls must still be present — extracting thinking shouldn't
	//     drop the tool invocation.
	if len(assistantMsg.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d, want 1", len(assistantMsg.ToolCalls))
	}
	if got, want := assistantMsg.ToolCalls[0].Function.Name, "search"; got != want {
		t.Fatalf("ToolCalls[0].Name = %q, want %q", got, want)
	}
}

func mustJSONBytes(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return json.RawMessage(b)
}

func TestTransformRequestVisionModelPassesImageContent(t *testing.T) {
	transformer := NewRequestTransformer()

	// User message with both text and an image
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"text","text":"What's in this image?"},
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}
				]`),
			},
		},
	}

	// Vision-capable model: images should be passed as image_url content parts
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "kimi-k2.6",
		Vision:  config.BoolPtr(true),
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	body, err := json.Marshal(openaiReq.Messages[0].Content)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !bytes.Contains(body, []byte(`"type":"image_url"`)) {
		t.Fatalf("vision model content missing image_url: %s", body)
	}
	if !bytes.Contains(body, []byte(`"data:image/png;base64,iVBORw0KGgo="`)) {
		t.Fatalf("vision model content missing image data URL: %s", body)
	}
	if !bytes.Contains(body, []byte(`"What's in this image?"`)) {
		t.Fatalf("vision model content missing text: %s", body)
	}
}

func TestTransformRequestNonVisionModelStripsImages(t *testing.T) {
	transformer := NewRequestTransformer()

	// User message with both text and an image
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"text","text":"What's in this image?"},
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}
				]`),
			},
		},
	}

	// Non-vision model: images should be replaced with [Image] placeholder
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-pro",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	content := openaiReq.Messages[0].ContentText()
	if !strings.Contains(content, "[Image]") {
		t.Fatalf("non-vision model content missing [Image] placeholder: %q", content)
	}
	if !strings.Contains(content, "What's in this image?") {
		t.Fatalf("non-vision model content missing original text: %q", content)
	}
	// Verify no image_url was sent
	body, err := json.Marshal(openaiReq.Messages[0].Content)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if bytes.Contains(body, []byte(`"type":"image_url"`)) {
		t.Fatalf("non-vision model should not contain image_url: %s", body)
	}
}

func TestTransformRequestNonVisionModelImageOnly(t *testing.T) {
	transformer := NewRequestTransformer()

	// User message with only an image, no text
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	content := openaiReq.Messages[0].ContentText()
	if got, want := content, "[Image]"; got != want {
		t.Fatalf("ContentText() = %q, want %q", got, want)
	}
}

func TestTransformRequestDeepSeekRewritesSystemRoleToUserMessage(t *testing.T) {
	t.Parallel()
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"task tools haven't been used recently"}]`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Should have 2 messages: user + rewritten system
	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	// The system message should be rewritten to a user message with <system-reminder> tags
	msg := openaiReq.Messages[1]
	if msg.Role != "user" {
		t.Fatalf("rewritten message role = %q, want %q", msg.Role, "user")
	}
	wantContent := "<system-reminder>\ntask tools haven't been used recently\n</system-reminder>"
	if got := msg.ContentText(); got != wantContent {
		t.Fatalf("rewritten content = %q, want %q", got, wantContent)
	}
}

func TestTransformRequestNonDeepSeekIgnoresSystemRoleMessages(t *testing.T) {
	t.Parallel()
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "qwen3.6-plus",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"a system reminder"}]`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "qwen3.6-plus"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Non-DeepSeek models should pass system messages to transformMessage for normal handling.
	// transformMessage's default case will produce a message with the original role.
	lastMsg := openaiReq.Messages[len(openaiReq.Messages)-1]
	if lastMsg.Role != "system" {
		t.Fatalf("non-DeepSeek system message role = %q, want %q (should pass through unchanged)", lastMsg.Role, "system")
	}
}

func TestTransformRequestDeepSeekDeduplicatesSystemMessage(t *testing.T) {
	t.Parallel()
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 256,
		System:    json.RawMessage(`"You are Claude. task tools haven't been used recently"`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			// This system message is a substring of the top-level system prompt — should be skipped
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"task tools haven't been used recently"}]`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Should have 2 messages: system (from top-level) + user
	// The duplicate system message should be dropped
	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	// First message should be the top-level system prompt
	if openaiReq.Messages[0].Role != "system" {
		t.Fatalf("first message role = %q, want %q", openaiReq.Messages[0].Role, "system")
	}
	// Second message should be the user message
	if openaiReq.Messages[1].Role != "user" {
		t.Fatalf("second message role = %q, want %q", openaiReq.Messages[1].Role, "user")
	}
	if got, want := openaiReq.Messages[1].ContentText(), "hello"; got != want {
		t.Fatalf("second message content = %q, want %q", got, want)
	}
}

func TestTransformRequestDeepSeekSkipsEmptySystemMessage(t *testing.T) {
	t.Parallel()
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			// Empty text blocks — should be skipped
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":""}]`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Should have exactly 1 message (just the user message)
	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}
}

func TestTransformRequestDeepSeekRewritesMultipleSystemMessages(t *testing.T) {
	t.Parallel()
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"first reminder"}]`)},
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"second reminder"}]`)},
			{Role: "assistant", Content: json.RawMessage(`"hi"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Should have 4 messages: 2 rewritten system + user + assistant
	if got, want := len(openaiReq.Messages), 4; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	// Both system messages should be rewritten to user with <system-reminder> tags
	for i, idx := range []int{0, 2} {
		msg := openaiReq.Messages[idx]
		if msg.Role != "user" {
			t.Fatalf("Messages[%d] role = %q, want %q", idx, msg.Role, "user")
		}
		wantSuffix := []string{"first reminder", "second reminder"}[i]
		got := msg.ContentText()
		if !strings.Contains(got, wantSuffix) {
			t.Fatalf("Messages[%d] content = %q, want it to contain %q", idx, got, wantSuffix)
		}
		if !strings.HasPrefix(got, "<system-reminder>") {
			t.Fatalf("Messages[%d] content = %q, want <system-reminder> prefix", idx, got)
		}
	}

	// The user and assistant messages should be in their original positions
	if openaiReq.Messages[1].Role != "user" || openaiReq.Messages[1].ContentText() != "hello" {
		t.Fatalf("Messages[1] expected user 'hello', got %q %q", openaiReq.Messages[1].Role, openaiReq.Messages[1].ContentText())
	}
	if openaiReq.Messages[3].Role != "assistant" || openaiReq.Messages[3].ContentText() != "hi" {
		t.Fatalf("Messages[3] expected assistant 'hi', got %q %q", openaiReq.Messages[3].Role, openaiReq.Messages[3].ContentText())
	}
}

func TestTransformRequestStandardModelIgnoresThinkingAndEffort(t *testing.T) {
	transformer := NewRequestTransformer()
	stream := true

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Stream:    &stream,
		Thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":2048}`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	// Standard model like qwen3.6-plus without explicit config
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "qwen3.6-plus",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("expected ReasoningEffort to be nil for standard model, got %v", *openaiReq.ReasoningEffort)
	}
	if openaiReq.Thinking != nil {
		t.Fatalf("expected Thinking to be nil for standard model, got %s", string(openaiReq.Thinking))
	}
}

func TestConstrainTemperature(t *testing.T) {
	tests := []struct {
		modelID string
		input   float64
		want    float64
	}{
		// kimi-k2.7-code forces temperature to 1.0
		{modelID: "kimi-k2.7-code", input: 0.7, want: 1.0},
		{modelID: "kimi-k2.7-code", input: 0.0, want: 1.0},
		{modelID: "kimi-k2.7-code", input: 1.5, want: 1.0},

		// Other kimi models are not constrained
		{modelID: "kimi-k2.6", input: 0.7, want: 0.7},
		{modelID: "kimi-k2.5", input: 0.5, want: 0.5},

		// Other models are not constrained
		{modelID: "minimax-m3", input: 0.7, want: 0.7},
		{modelID: "deepseek-v4-pro", input: 0.5, want: 0.5},
		{modelID: "glm-5.1", input: 0.3, want: 0.3},
		{modelID: "qwen3.7-plus", input: 0.9, want: 0.9},
	}

	for _, tt := range tests {
		t.Run(tt.modelID+"/"+fmt.Sprint(tt.input), func(t *testing.T) {
			if got := constrainTemperature(tt.modelID, tt.input); got != tt.want {
				t.Errorf("constrainTemperature(%q, %f) = %f, want %f", tt.modelID, tt.input, got, tt.want)
			}
		})
	}
}

// TestTransformTools_HandlesWhitespaceNullSchema guards against a panic on
// valid JSON that unmarshals to a nil map (e.g. " null " with decorative
// whitespace). The fix is to fall back to the default schema when schemaObj
// is nil after Unmarshal.
func TestTransformTools_HandlesWhitespaceNullSchema(t *testing.T) {
	transformer := NewRequestTransformer()
	tools := []types.Tool{
		{Name: "Bash", Description: "decorative null", InputSchema: json.RawMessage(` null `)},
	}

	result := transformer.transformTools(tools)
	if got, want := len(result), 1; got != want {
		t.Fatalf("len(result) = %d, want %d (whitespace-null schema should fall back, not panic)", got, want)
	}

	params := string(result[0].Function.Parameters)
	if !strings.Contains(params, `"type":"object"`) {
		t.Fatalf("whitespace-null schema should fall back to default object schema: %s", params)
	}
	if !strings.Contains(params, `"properties":{}`) {
		t.Fatalf("whitespace-null schema should fall back to default properties: %s", params)
	}
}

func TestTransformTools_SkipsEmptyName(t *testing.T) {
	transformer := NewRequestTransformer()
	tools := []types.Tool{
		{Name: "", Description: "empty name", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "Bash", Description: "valid tool", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	result := transformer.transformTools(tools)
	if got, want := len(result), 1; got != want {
		t.Fatalf("len(result) = %d, want %d (empty-name tool should be skipped)", got, want)
	}
	if got, want := result[0].Function.Name, "Bash"; got != want {
		t.Fatalf("result[0].Name = %q, want %q", got, want)
	}
}

func TestTransformTools_SkipsWhitespaceOnlyName(t *testing.T) {
	transformer := NewRequestTransformer()
	tools := []types.Tool{
		{Name: "   ", Description: "whitespace name", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "Bash", Description: "valid tool", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	result := transformer.transformTools(tools)
	if got, want := len(result), 1; got != want {
		t.Fatalf("len(result) = %d, want %d (whitespace-name tool should be skipped)", got, want)
	}
}

func TestTransformTools_FillsEmptySchema(t *testing.T) {
	transformer := NewRequestTransformer()
	tools := []types.Tool{
		{Name: "Bash", Description: "no schema", InputSchema: nil},
	}

	result := transformer.transformTools(tools)
	if got, want := len(result), 1; got != want {
		t.Fatalf("len(result) = %d, want %d", got, want)
	}

	params := string(result[0].Function.Parameters)
	if !strings.Contains(params, `"type":"object"`) {
		t.Fatalf("parameters missing type=object: %s", params)
	}
	if !strings.Contains(params, `"additionalProperties":false`) {
		t.Fatalf("parameters missing additionalProperties=false: %s", params)
	}
}

func TestTransformTools_FillsNullSchema(t *testing.T) {
	transformer := NewRequestTransformer()
	tools := []types.Tool{
		{Name: "Bash", Description: "null schema", InputSchema: json.RawMessage(`null`)},
	}

	result := transformer.transformTools(tools)
	if got, want := len(result), 1; got != want {
		t.Fatalf("len(result) = %d, want %d", got, want)
	}

	params := string(result[0].Function.Parameters)
	if !strings.Contains(params, `"type":"object"`) {
		t.Fatalf("null schema should become type=object: %s", params)
	}
}

func TestTransformTools_FillsEmptyObjectSchema(t *testing.T) {
	transformer := NewRequestTransformer()
	tools := []types.Tool{
		{Name: "Bash", Description: "empty object schema", InputSchema: json.RawMessage(`{}`)},
	}

	result := transformer.transformTools(tools)
	if got, want := len(result), 1; got != want {
		t.Fatalf("len(result) = %d, want %d", got, want)
	}

	params := string(result[0].Function.Parameters)
	if !strings.Contains(params, `"type":"object"`) {
		t.Fatalf("empty object schema should get type=object: %s", params)
	}
	if !strings.Contains(params, `"additionalProperties":false`) {
		t.Fatalf("empty object schema should get additionalProperties=false: %s", params)
	}
}

func TestTransformTools_FillsMissingType(t *testing.T) {
	transformer := NewRequestTransformer()
	tools := []types.Tool{
		{Name: "Search", Description: "schema without type", InputSchema: json.RawMessage(`{"properties":{"query":{"type":"string"}}}`)},
	}

	result := transformer.transformTools(tools)
	if got, want := len(result), 1; got != want {
		t.Fatalf("len(result) = %d, want %d", got, want)
	}

	params := string(result[0].Function.Parameters)
	if !strings.Contains(params, `"type":"object"`) {
		t.Fatalf("schema missing type should get type=object: %s", params)
	}
	if !strings.Contains(params, `"query"`) {
		t.Fatalf("existing properties should be preserved: %s", params)
	}
}

func TestTransformTools_FillsMissingProperties(t *testing.T) {
	transformer := NewRequestTransformer()
	tools := []types.Tool{
		{Name: "NoOp", Description: "schema without properties", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	result := transformer.transformTools(tools)
	if got, want := len(result), 1; got != want {
		t.Fatalf("len(result) = %d, want %d", got, want)
	}

	params := string(result[0].Function.Parameters)
	if !strings.Contains(params, `"properties"`) {
		t.Fatalf("schema missing properties should get properties={}: %s", params)
	}
}

func TestTransformTools_RecoversFromInvalidJSON(t *testing.T) {
	transformer := NewRequestTransformer()
	tools := []types.Tool{
		{Name: "Bash", Description: "malformed JSON", InputSchema: json.RawMessage(`{invalid`)},
	}

	result := transformer.transformTools(tools)
	if got, want := len(result), 1; got != want {
		t.Fatalf("len(result) = %d, want %d (malformed schema should be replaced, not skipped)", got, want)
	}

	params := string(result[0].Function.Parameters)
	if !strings.Contains(params, `"type":"object"`) {
		t.Fatalf("malformed schema should be replaced with valid schema: %s", params)
	}
}

func TestTransformTools_PreservesValidSchema(t *testing.T) {
	transformer := NewRequestTransformer()
	originalSchema := json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string","description":"The command"}},"required":["cmd"]}`)
	tools := []types.Tool{
		{Name: "Bash", Description: "run a command", InputSchema: originalSchema},
	}

	result := transformer.transformTools(tools)
	if got, want := len(result), 1; got != want {
		t.Fatalf("len(result) = %d, want %d", got, want)
	}

	params := string(result[0].Function.Parameters)
	if !strings.Contains(params, `"cmd"`) {
		t.Fatalf("valid schema properties should be preserved: %s", params)
	}
	if !strings.Contains(params, `"required"`) {
		t.Fatalf("valid schema required should be preserved: %s", params)
	}
	if !strings.Contains(params, `"type":"string"`) {
		t.Fatalf("valid schema nested type should be preserved: %s", params)
	}
}

func TestTransformTools_PreservesAdditionalPropertiesWhenSet(t *testing.T) {
	transformer := NewRequestTransformer()
	tools := []types.Tool{
		{Name: "Flexible", Description: "allows extra props", InputSchema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"}},"additionalProperties":true}`)},
	}

	result := transformer.transformTools(tools)
	if got, want := len(result), 1; got != want {
		t.Fatalf("len(result) = %d, want %d", got, want)
	}

	params := string(result[0].Function.Parameters)
	if !strings.Contains(params, `"additionalProperties":true`) {
		t.Fatalf("existing additionalProperties should be preserved: %s", params)
	}
}

// The family rules in request.go are written against model names, but upstreams
// namespace their ids differently: OpenCode ships them flat ("deepseek-v4-pro"),
// CommandCode ships them vendor-prefixed ("deepseek/deepseek-v4.1-flash") and
// WorkBuddy ships them realm-prefixed ("global:deepseek-v4.1-flash"). A prefix
// must not switch a family rule off, so every spelling has to produce the same
// family-specific payload.
//
// Each spelling is pinned to the behaviour it should get rather than compared
// with the others: a parity check also passes when every family rule is
// switched off symmetrically, and it cannot tell an intended difference from an
// accidental one.
func TestModelFamilyRulesStillDiscriminate(t *testing.T) {
	transformer := NewRequestTransformer()
	payload := func(modelID string, messages []types.Message) map[string]json.RawMessage {
		t.Helper()
		temperature := 0.5
		req := &types.MessageRequest{
			Model:       "alias",
			MaxTokens:   256,
			Temperature: &temperature,
			Thinking:    json.RawMessage(`{"type":"enabled","budget_tokens":4096}`),
			System: json.RawMessage(`[
				{"type":"text","text":"system prompt","cache_control":{"type":"ephemeral"}}
			]`),
			Messages: messages,
		}
		out, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: modelID})
		if err != nil {
			t.Fatalf("TransformRequest(%q) error = %v", modelID, err)
		}
		encoded, err := json.Marshal(out)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		return fields
	}

	firstTurn := []types.Message{{Role: "user", Content: json.RawMessage(`"explain"`)}}
	plainHistory := []types.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`"hi"`)},
		{Role: "user", Content: json.RawMessage(`"again"`)},
	}
	// Claude Code injects system reminders mid-conversation; DeepSeek reorders
	// every system message to the front, which would otherwise shift the whole
	// history and invalidate the prefix cache.
	midturnSystem := []types.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`"hi"`)},
		{Role: "system", Content: json.RawMessage(`"task tools haven't been used recently"`)},
		{Role: "user", Content: json.RawMessage(`"continue"`)},
	}

	spellings := []struct{ deepseek, kimi, kimiCode string }{
		{"deepseek/deepseek-v4.1-flash", "moonshotai/Kimi-K2.6", "moonshotai/Kimi-K2.7-Code"},
		{"deepseek-v4.1-flash", "kimi-k2.6", "kimi-k2.7-code"},
		{"global:deepseek-v4.1-flash", "global:kimi-k2.6", "global:kimi-k2.7-code"},
		{"cn:deepseek-v4.1-flash", "cn:kimi-k2.6", "cn:kimi-k2.7-code"},
	}

	for _, spelling := range spellings {
		t.Run(spelling.deepseek, func(t *testing.T) {
			// DeepSeek keeps thinking mode and the cache_control the prefix cache
			// needs. The budget is the control it honours, so no derived effort
			// may ride along: that would be a second, contradicting answer.
			deepseek := payload(spelling.deepseek, firstTurn)
			if got, want := string(deepseek["thinking"]), `{"type":"enabled","budget_tokens":4096}`; got != want {
				t.Fatalf("deepseek first turn lost thinking mode: thinking=%s", got)
			}
			// A realm prefix means the request reaches the model through the
			// WorkBuddy gateway, which accepts a budget and then ignores it -
			// measured 2026-10-26, 8192 and 64000 gave the same reasoning-token
			// count while the level moved it. There the level must ride along
			// or the caller has no way to steer. A bare or vendor-prefixed id
			// reaches the vendor directly, where the budget is the control.
			if strings.Contains(spelling.deepseek, ":") {
				if deepseek["reasoning_effort"] == nil {
					t.Fatalf("gateway-reached deepseek lost the level that actually steers it: %s", deepseek["thinking"])
				}
			} else if deepseek["reasoning_effort"] != nil {
				t.Fatalf("direct deepseek was sent both a budget and a derived effort: effort=%s", deepseek["reasoning_effort"])
			}
			if !bytes.Contains(deepseek["messages"], []byte("cache_control")) {
				t.Fatalf("deepseek lost cache_control, so the prefix cache cannot be anchored: %s", deepseek["messages"])
			}
			// On a turn whose assistant messages carry no thinking it must send the
			// explicit "disabled" form instead, or upstream rejects the history.
			if got, want := string(payload(spelling.deepseek, plainHistory)["thinking"]), `{"type":"disabled"}`; got != want {
				t.Fatalf("deepseek history guard did not engage: thinking=%s", got)
			}
			// A mid-conversation system message must be lifted to the front.
			var reordered []struct {
				Role string `json:"role"`
			}
			mid := payload(spelling.deepseek, midturnSystem)["messages"]
			if err := json.Unmarshal(mid, &reordered); err != nil {
				t.Fatalf("unmarshal messages: %v", err)
			}
			if len(reordered) == 0 || reordered[0].Role != "system" {
				t.Fatalf("deepseek did not lift the system reminder to the front: %s", mid)
			}
		})

		t.Run(spelling.kimi, func(t *testing.T) {
			// Moonshot accepts neither the thinking field nor cache_control.
			kimi := payload(spelling.kimi, firstTurn)
			if kimi["thinking"] != nil || kimi["reasoning_effort"] != nil {
				t.Fatalf("kimi was sent thinking/effort it rejects: thinking=%s effort=%s", kimi["thinking"], kimi["reasoning_effort"])
			}
			if bytes.Contains(kimi["messages"], []byte("cache_control")) {
				t.Fatalf("kimi was sent cache_control it rejects: %s", kimi["messages"])
			}
			if got, want := string(payload(spelling.kimiCode, firstTurn)["temperature"]), "1"; got != want {
				t.Fatalf("kimi-k2.7-code temperature = %s, want 1", got)
			}
		})
	}
}

// One decision must not be expressed twice. When the upstream accepts a
// thinking budget, the budget is sent and no derived effort is added; when it
// only understands levels, the level is derived. Sending both hands the
// upstream two controls that can disagree, and the derived one is a lossy
// restatement of a number the request already carried precisely.
func TestThinkingBudgetAndEffortAreNeverSentTogether(t *testing.T) {
	transform := func(modelID string, thinking json.RawMessage) map[string]any {
		t.Helper()
		req := &types.MessageRequest{
			Model:     modelID,
			MaxTokens: 4096,
			Thinking:  thinking,
			Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"go"`)}},
		}
		out, err := NewRequestTransformer().TransformRequest(req, config.ModelConfig{ModelID: modelID})
		if err != nil {
			t.Fatalf("%s: %v", modelID, err)
		}
		raw, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		return fields
	}
	budget := json.RawMessage(`{"type":"enabled","budget_tokens":8192}`)

	// DeepSeek accepts thinking.budget_tokens: budget only.
	deepseek := transform("deepseek/deepseek-v4.1-flash", budget)
	if deepseek["thinking"] == nil {
		t.Error("deepseek lost the budget the client stated")
	}
	if deepseek["reasoning_effort"] != nil {
		t.Errorf("deepseek was sent a derived effort (%v) alongside its budget", deepseek["reasoning_effort"])
	}

	// An OpenAI reasoning model does not take a budget: level only.
	openai := transform("o3-mini", budget)
	if openai["thinking"] != nil {
		t.Errorf("o3-mini was sent a thinking field it does not accept: %v", openai["thinking"])
	}
	if openai["reasoning_effort"] == nil {
		t.Error("o3-mini lost the reasoning level; it has no other way to express intent")
	}
}

// WorkBuddy's wire ids carry a realm prefix ("global:deepseek-v4.1-flash").
// Every family rule below is written against the bare name, so before
// BareModelID existed a realm-prefixed model matched none of them: the client's
// thinking config was dropped, the gateway substituted its own default effort,
// and nothing in the outbound body said the caller had asked for anything.
func TestTransformRequestSeesThroughTheRealmPrefix(t *testing.T) {
	tr := NewRequestTransformer()
	model := config.ModelConfig{ModelID: "global:deepseek-v4.1-flash"}
	firstTurn := []types.Message{
		{Role: "user", Content: json.RawMessage(`"solve this carefully"`)},
	}

	budget := json.RawMessage(`{"type":"enabled","budget_tokens":63999}`)
	openaiReq, err := tr.TransformRequest(&types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Thinking:  budget,
		Messages:  firstTurn,
	}, model)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}
	if got, want := string(openaiReq.Thinking), string(budget); got != want {
		t.Fatalf("Thinking = %s, want the client's budget forwarded (%s)", got, want)
	}

	// The disabled form is the one that changes what the model does, so it is
	// pinned separately: dropping it turns thinking on, which is the opposite
	// of what the caller asked for.
	openaiReq, err = tr.TransformRequest(&types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Thinking:  json.RawMessage(`{"type":"disabled"}`),
		Messages:  firstTurn,
	}, model)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

// Only a known realm is a realm. A colon that belongs to the model's own name
// must not be truncated into a different model's family.
func TestTransformRequestDoesNotStripAnUnknownPrefix(t *testing.T) {
	tr := NewRequestTransformer()

	openaiReq, err := tr.TransformRequest(&types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":4096}`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}, config.ModelConfig{ModelID: "custom:deepseek-v4.1-flash"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}
	if len(openaiReq.Thinking) != 0 {
		t.Fatalf("Thinking = %s, want nothing for a model whose family is unknown", openaiReq.Thinking)
	}
}

// One decision, one control. `output_config.effort` used to be written after
// resolveThinkingAndEffort had already set `thinking`, so a request carrying
// both produced a body with both - two answers to one question, which gateways
// such as OpenCode Go reject with 400, and which let the two disagree (a 8192
// budget reads "medium" by this proxy's ladder while the stated level says
// "high").
//
// The stated level wins because it is the caller's own words; the budget is
// this proxy's arithmetic. Disabling thinking outranks both.
func TestStatedEffortReplacesTheBudget(t *testing.T) {
	transform := func(modelID, thinking, outputConfig string) map[string]any {
		t.Helper()
		req := &types.MessageRequest{
			Model:     "claude-x",
			MaxTokens: 4096,
			Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"go"`)}},
		}
		if thinking != "" {
			req.Thinking = json.RawMessage(thinking)
		}
		if outputConfig != "" {
			req.OutputConfig = json.RawMessage(outputConfig)
		}
		out, err := NewRequestTransformer().TransformRequest(req, config.ModelConfig{ModelID: modelID})
		if err != nil {
			t.Fatalf("%s: %v", modelID, err)
		}
		raw, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		return fields
	}

	budget := `{"type":"enabled","budget_tokens":8192}`
	high := `{"effort":"high"}`

	// Budget plus a stated level: the level survives, the budget does not.
	both := transform("deepseek/deepseek-v4.1-flash", budget, high)
	if both["thinking"] != nil {
		t.Errorf("thinking was forwarded alongside a stated effort: %v", both["thinking"])
	}
	if got, want := both["reasoning_effort"], "high"; got != want {
		t.Errorf("reasoning_effort = %v, want %v", got, want)
	}

	// Disabled plus a stated level: the disable wins, no level is sent.
	off := transform("deepseek/deepseek-v4.1-flash", `{"type":"disabled"}`, high)
	if off["reasoning_effort"] != nil {
		t.Errorf("a disabled request still carried effort %v", off["reasoning_effort"])
	}
	if got, want := mustJSON(t, off["thinking"]), `{"type":"disabled"}`; got != want {
		t.Errorf("thinking = %s, want %s", got, want)
	}

	// A stated level must not reach a family measured to reject the field.
	if kimi := transform("moonshotai/Kimi-K2.6", "", high); kimi["reasoning_effort"] != nil {
		t.Errorf("kimi was sent effort it rejects: %v", kimi["reasoning_effort"])
	}

	// The budget-only path is unchanged: DeepSeek keeps its budget.
	only := transform("deepseek/deepseek-v4.1-flash", budget, "")
	if only["reasoning_effort"] != nil {
		t.Errorf("budget-only request gained an effort: %v", only["reasoning_effort"])
	}
	if only["thinking"] == nil {
		t.Error("budget-only request lost its thinking block")
	}
}

// Three ways a reasoning field used to reach an upstream that could not read
// it, or fail to reach one that could.
func TestReasoningFieldsAreValidatedBeforeTheWire(t *testing.T) {
	transform := func(modelID, thinking, outputConfig string, cfg config.ModelConfig) (map[string]any, error) {
		t.Helper()
		req := &types.MessageRequest{
			Model:     "claude-x",
			MaxTokens: 4096,
			Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"go"`)}},
		}
		if thinking != "" {
			req.Thinking = json.RawMessage(thinking)
		}
		if outputConfig != "" {
			req.OutputConfig = json.RawMessage(outputConfig)
		}
		cfg.ModelID = modelID
		out, err := NewRequestTransformer().TransformRequest(req, cfg)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		return fields, nil
	}

	// "adaptive" is an Anthropic mode. DeepSeek's thinking object defines only
	// enabled/disabled, so forwarding the word verbatim asks it to interpret
	// something it has never seen.
	adaptive, err := transform("deepseek/deepseek-v4.1-flash", `{"type":"adaptive"}`, "", config.ModelConfig{})
	if err != nil {
		t.Fatalf("adaptive thinking: %v", err)
	}
	if got, want := mustJSON(t, adaptive["thinking"]), `{"type":"enabled"}`; got != want {
		t.Errorf("deepseek received thinking %s, want %s", got, want)
	}

	// A caller may state "let the model decide" as an effort. That is a mode,
	// not a rung: it must neither fail the request nor invent a level.
	auto, err := transform("deepseek/deepseek-v4.1-flash", "", `{"effort":"adaptive"}`, config.ModelConfig{})
	if err != nil {
		t.Fatalf("adaptive effort was rejected: %v", err)
	}
	if auto["reasoning_effort"] != nil {
		t.Errorf("adaptive effort produced a level: %v", auto["reasoning_effort"])
	}

	// A configuration typo must not become an upstream field: the upstream
	// would answer 400 and nothing in the request would point back at config.
	typo, err := transform("deepseek/deepseek-v4.1-flash", "", "",
		config.ModelConfig{ReasoningEffort: "very high"})
	if err != nil {
		t.Fatalf("typo config: %v", err)
	}
	if typo["reasoning_effort"] != nil {
		t.Errorf("an invalid configured effort reached the wire: %v", typo["reasoning_effort"])
	}

	// A valid configured effort still works.
	ok, err := transform("deepseek/deepseek-v4.1-flash", "", "",
		config.ModelConfig{ReasoningEffort: "low"})
	if err != nil {
		t.Fatalf("valid config: %v", err)
	}
	if got, want := ok["reasoning_effort"], "low"; got != want {
		t.Errorf("reasoning_effort = %v, want %v", got, want)
	}
}

// The DeepSeek history guard outranks a stated level.
//
// DeepSeek validates reasoning_content on every assistant message in thinking
// mode, so a history without it must run with thinking off - it is a refusal,
// not a preference. Ordering the stated level above the guard produced a
// payload that set only reasoning_effort and left thinking on, which is exactly
// the combination the guard exists to prevent.
func TestHistoryGuardOutranksAStatedEffort(t *testing.T) {
	history := []types.Message{
		{Role: "user", Content: json.RawMessage(`"hi"`)},
		{Role: "assistant", Content: json.RawMessage(`"hello"`)},
		{Role: "user", Content: json.RawMessage(`"go on"`)},
	}
	req := &types.MessageRequest{
		Model:        "claude-x",
		MaxTokens:    4096,
		Messages:     history,
		OutputConfig: json.RawMessage(`{"effort":"high"}`),
	}
	out, err := NewRequestTransformer().TransformRequest(req, config.ModelConfig{ModelID: "deepseek/deepseek-v4.1-flash"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if got, want := mustJSON(t, fields["thinking"]), `{"type":"disabled"}`; got != want {
		t.Errorf("thinking = %s, want %s (guard must win)", got, want)
	}
	if fields["reasoning_effort"] != nil {
		t.Errorf("a guarded request still carried effort %v", fields["reasoning_effort"])
	}
}

// "Stop thinking" and "think as little as possible" are different requests.
//
// A model whose ladder has no "none" cannot be told to stop through a level:
// returning its lowest rung would answer "think a little" where the caller said
// "stop". Such a model is told through the switch instead. A model with no
// switch at all (the GPT family) has only its ladder, so the lowest rung is the
// closest honest answer there.
func TestOffIsExpressedThroughTheSwitchNotALevel(t *testing.T) {
	firstTurn := []types.Message{{Role: "user", Content: json.RawMessage(`"go"`)}}
	transform := func(modelID, effort string) map[string]any {
		t.Helper()
		req := &types.MessageRequest{
			Model: "claude-x", MaxTokens: 4096, Messages: firstTurn,
			OutputConfig: json.RawMessage(`{"effort":"` + effort + `"}`),
		}
		out, err := NewRequestTransformer().TransformRequest(req, config.ModelConfig{ModelID: modelID})
		if err != nil {
			t.Fatalf("%s/%s: %v", modelID, effort, err)
		}
		raw, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		return fields
	}

	// DeepSeek has a switch, so "none" uses it and no level is invented.
	off := transform("deepseek/deepseek-v4.1-flash", "none")
	if got, want := mustJSON(t, off["thinking"]), `{"type":"disabled"}`; got != want {
		t.Errorf("deepseek none: thinking = %s, want %s", got, want)
	}
	if off["reasoning_effort"] != nil {
		t.Errorf("deepseek none produced a level: %v", off["reasoning_effort"])
	}

	// "minimal" is not "off": it still resolves to the weakest real rung.
	minimal := transform("deepseek/deepseek-v4.1-flash", "minimal")
	if got, want := minimal["reasoning_effort"], "low"; got != want {
		t.Errorf("deepseek minimal = %v, want %v", got, want)
	}

	// No switch anywhere: the lowest rung is the only answer available.
	noSwitch := transform("gpt-6-astra", "none")
	if got, want := noSwitch["reasoning_effort"], "low"; got != want {
		t.Errorf("gpt-6 none = %v, want the lowest rung %v", got, want)
	}
}

// WorkBuddy publishes a different effort ladder per realm for the same model
// id, and a kimi reached through its gateway is a different endpoint from
// api.moonshot.ai. Both facts live in the id's prefix, so the capability
// lookup reads it.
func TestRealmDecidesTheLadderAndTheDenyList(t *testing.T) {
	firstTurn := []types.Message{{Role: "user", Content: json.RawMessage(`"go"`)}}
	transform := func(modelID, effort string) map[string]any {
		t.Helper()
		req := &types.MessageRequest{
			Model: "claude-x", MaxTokens: 4096, Messages: firstTurn,
			OutputConfig: json.RawMessage(`{"effort":"` + effort + `"}`),
		}
		out, err := NewRequestTransformer().TransformRequest(req, config.ModelConfig{ModelID: modelID})
		if err != nil {
			t.Fatalf("%s/%s: %v", modelID, effort, err)
		}
		raw, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		return fields
	}

	// The international gateway advertises five rungs; xhigh survives there.
	if got := transform("global:deepseek-v4.1-flash", "xhigh")["reasoning_effort"]; got != "xhigh" {
		t.Errorf("global deepseek xhigh = %v, want the level to survive", got)
	}
	// The domestic gateway advertises three, so xhigh folds to high.
	if got := transform("cn:deepseek-v4.1-flash", "xhigh")["reasoning_effort"]; got != "high" {
		t.Errorf("cn deepseek xhigh = %v, want the nearest rung high", got)
	}
	// GLM-5.2 documents high/xhigh; a low request snaps up, not down to max.
	if got := transform("global:glm-5.2", "low")["reasoning_effort"]; got != "high" {
		t.Errorf("glm-5.2 low = %v, want the nearest rung high", got)
	}

	// A kimi through the gateway accepts a level; the bare id is the Moonshot
	// API, which rejects the field outright.
	if got := transform("global:kimi-k3", "high")["reasoning_effort"]; got != "high" {
		t.Errorf("gateway kimi-k3 = %v, want the level forwarded", got)
	}
	if got := transform("moonshotai/Kimi-K2.6", "high")["reasoning_effort"]; got != nil {
		t.Errorf("bare Moonshot got effort %v, want none", got)
	}
}

// On the WorkBuddy gateway a budget saturates below the top rung, so the level
// has to ride along.
//
// Measured 2026-10-26 against workbuddy2api-panel, same reasoning-heavy prompt,
// three samples per cell: budget 8192 / 32000 / 64000 averaged 216 / 306 / 295
// reasoning tokens, while reasoning_effort low / high / max averaged 250 / 305
// / 462. The budget is honoured and then flattens - no budget value reached
// max, so a caller who states a large budget still needs the level.
//
// The thinking block rides along because it carries the on/off switch: the
// same request with thinking disabled produced zero reasoning tokens.
//
// A bare id is not behind that gateway, so nothing changes for it.
func TestEffortDrivenGatewayGetsALevelFromTheBudget(t *testing.T) {
	firstTurn := []types.Message{{Role: "user", Content: json.RawMessage(`"go"`)}}
	transform := func(modelID string, budget int) map[string]any {
		t.Helper()
		req := &types.MessageRequest{
			Model: "claude-x", MaxTokens: 4096, Messages: firstTurn,
			Thinking: json.RawMessage(`{"type":"enabled","budget_tokens":` + itoa(budget) + `}`),
		}
		out, err := NewRequestTransformer().TransformRequest(req, config.ModelConfig{ModelID: modelID})
		if err != nil {
			t.Fatalf("%s: %v", modelID, err)
		}
		raw, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		return fields
	}

	small := transform("global:deepseek-v4.1-flash", 8192)
	if got, want := small["reasoning_effort"], "medium"; got != want {
		t.Errorf("8192 budget produced effort %v, want %v", got, want)
	}
	large := transform("global:deepseek-v4.1-flash", 64000)
	if got, want := large["reasoning_effort"], "max"; got != want {
		t.Errorf("64000 budget produced effort %v, want %v", got, want)
	}
	if small["reasoning_effort"] == large["reasoning_effort"] {
		t.Error("two very different budgets produced the same level, so the budget is still being ignored")
	}
	// The switch still rides along: it is the only thing that turns thinking off.
	if small["thinking"] == nil {
		t.Error("the thinking block was dropped, so the caller can no longer turn thinking off")
	}

	// A bare id is not behind the gateway, so the budget stays the control.
	if got := transform("deepseek-v4-pro", 8192)["reasoning_effort"]; got != nil {
		t.Errorf("bare deepseek gained a level %v, want the budget alone", got)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
