package transformer

import (
	"encoding/json"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestAnthropicForModel_PreservesContentAndSetsStream(t *testing.T) {
	streamIn := true
	req := &types.MessageRequest{
		Model:     "claude-opus-4-8",
		System:    json.RawMessage(`"Line one\nLine two\nLine three"`),
		MaxTokens: 100,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"Hello\nWorld"`)},
		},
		Stream: &streamIn,
	}

	// A non-streaming call must not forward stream:true, and the model ID must
	// be rewritten to the routed model.
	out := AnthropicForModel(req, config.ModelConfig{ModelID: "minimax-m3"}, false)

	if _, err := json.Marshal(out); err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if out.Model != "minimax-m3" {
		t.Errorf("model = %q, want minimax-m3", out.Model)
	}
	if out.Stream != nil {
		t.Errorf("stream = %v, want nil for a non-streaming call", *out.Stream)
	}
	if got := out.SystemText(); got != "Line one\nLine two\nLine three" {
		t.Errorf("system text mismatch: got %q", got)
	}
	blocks := out.Messages[0].ContentBlocks()
	if len(blocks) != 1 || blocks[0].Text != "Hello\nWorld" {
		t.Errorf("unexpected content blocks: %+v", blocks)
	}
	// The caller's request must not be mutated.
	if req.Model != "claude-opus-4-8" || req.Stream == nil {
		t.Errorf("source request was mutated: model=%q stream=%v", req.Model, req.Stream)
	}
}

func TestAnthropicForModel_StreamingSetsStreamTrue(t *testing.T) {
	req := &types.MessageRequest{Model: "claude-opus-4-8", MaxTokens: 100}

	out := AnthropicForModel(req, config.ModelConfig{ModelID: "minimax-m3"}, true)

	if out.Stream == nil || !*out.Stream {
		t.Fatalf("stream = %v, want true", out.Stream)
	}
}

func TestAnthropicToResponses_SystemPromptWithNewline(t *testing.T) {
	req := &types.MessageRequest{
		Model:     "gpt-5",
		System:    json.RawMessage(`"Line one\nLine two"`),
		MaxTokens: 100,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"Hello\nWorld"`)},
		},
	}

	responsesReq, err := AnthropicToResponses(req, config.ModelConfig{ModelID: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}

	// The original bug: content was built by wrapping the raw string in quotes
	// instead of JSON-encoding it, so any newline produced invalid JSON.
	if _, err := json.Marshal(responsesReq); err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	if len(responsesReq.Input) != 2 {
		t.Fatalf("input count mismatch: got %d, want 2", len(responsesReq.Input))
	}

	var systemPrompt string
	if err := json.Unmarshal(responsesReq.Input[0].Content, &systemPrompt); err != nil {
		t.Fatalf("system prompt content was not valid JSON: %v", err)
	}
	if systemPrompt != "Line one\nLine two" {
		t.Fatalf("system prompt mismatch: got %q", systemPrompt)
	}

	var messageContent string
	if err := json.Unmarshal(responsesReq.Input[1].Content, &messageContent); err != nil {
		t.Fatalf("message content was not valid JSON: %v", err)
	}
	if messageContent != "Hello\nWorld" {
		t.Fatalf("message content mismatch: got %q", messageContent)
	}
}

// Gemini splits by generation: 3.x takes a discrete level, 2.5 takes a token
// budget. Sending the wrong one, or both, is the conflict Gemini documents as a
// 400 - so the capability decides which field this target gets, and a stated
// budget is clamped to the range the specific model publishes.
func TestGeminiCarriesTheRightControlForItsGeneration(t *testing.T) {
	firstTurn := []types.Message{{Role: "user", Content: json.RawMessage(`"go"`)}}

	// 3.x: level-only. A stated level is clamped to the published rungs.
	level := &types.MessageRequest{
		Model: "claude-x", MaxTokens: 4096,
		OutputConfig: json.RawMessage(`{"effort":"xhigh"}`),
		Messages:     firstTurn,
	}
	got := mustGeminiRequest(t, level, config.ModelConfig{ModelID: "gemini-3.8-flash"})
	if got.GenerationConfig.ThinkingConfig == nil || got.GenerationConfig.ThinkingConfig.ThinkingLevel == nil {
		t.Fatal("gemini 3.x lost the level")
	}
	if v := *got.GenerationConfig.ThinkingConfig.ThinkingLevel; v != "high" {
		t.Fatalf("thinkingLevel = %q, want the nearest rung high", v)
	}
	if got.GenerationConfig.ThinkingConfig.ThinkingBudget != nil {
		t.Fatal("gemini 3.x was sent a budget as well, which it rejects")
	}
	// A level request implies the caller wants to see the reasoning; without
	// includeThoughts the model may think and report nothing, which reads
	// downstream as "no reasoning" rather than "not requested".
	if got.GenerationConfig.ThinkingConfig.IncludeThoughts == nil || !*got.GenerationConfig.ThinkingConfig.IncludeThoughts {
		t.Error("gemini 3.x level request did not ask for thought summaries")
	}

	// 2.5-pro: budget-only, clamped to its published ceiling.
	pro := &types.MessageRequest{
		Model: "claude-x", MaxTokens: 4096,
		Thinking: json.RawMessage(`{"type":"enabled","budget_tokens":100000}`),
		Messages: firstTurn,
	}
	got = mustGeminiRequest(t, pro, config.ModelConfig{ModelID: "gemini-2.5-pro"})
	if got.GenerationConfig.ThinkingConfig == nil || got.GenerationConfig.ThinkingConfig.ThinkingBudget == nil {
		t.Fatal("gemini 2.5 lost the budget")
	}
	if v := *got.GenerationConfig.ThinkingConfig.ThinkingBudget; v != 32768 {
		t.Fatalf("thinkingBudget = %d, want the 2.5-pro ceiling 32768", v)
	}
	if got.GenerationConfig.ThinkingConfig.ThinkingLevel != nil {
		t.Fatal("gemini 2.5 was sent a level as well, which it rejects")
	}

	// flash-lite publishes a different floor, and the floor applies.
	small := &types.MessageRequest{
		Model: "claude-x", MaxTokens: 4096,
		Thinking: json.RawMessage(`{"type":"enabled","budget_tokens":100}`),
		Messages: firstTurn,
	}
	got = mustGeminiRequest(t, small, config.ModelConfig{ModelID: "gemini-2.5-flash-lite"})
	if v := *got.GenerationConfig.ThinkingConfig.ThinkingBudget; v != 512 {
		t.Fatalf("thinkingBudget = %d, want the flash-lite floor 512", v)
	}

	// Saying nothing leaves the field absent rather than inventing one.
	plain := &types.MessageRequest{Model: "claude-x", MaxTokens: 4096, Messages: firstTurn}
	if got := mustGeminiRequest(t, plain, config.ModelConfig{ModelID: "gemini-2.5-pro"}); got.GenerationConfig.ThinkingConfig != nil {
		t.Fatalf("an unstated budget produced thinkingConfig: %+v", got.GenerationConfig.ThinkingConfig)
	}
}
