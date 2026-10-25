package transformer

import (
	"encoding/json"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
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

// Gemini was the one target that received nothing at all about reasoning: the
// normalised request already carried the budget and the derived level, but the
// bridge never read them, so a request that asked for deep thinking reached the
// model with no thinkingConfig and the model used its default.
func TestGeminiCarriesTheThinkingBudget(t *testing.T) {
	firstTurn := []types.Message{{Role: "user", Content: json.RawMessage(`"go"`)}}

	budget := &types.MessageRequest{
		Model: "claude-x", MaxTokens: 4096,
		Thinking: json.RawMessage(`{"type":"enabled","budget_tokens":8192}`),
		Messages: firstTurn,
	}
	got := AnthropicToGemini(budget, config.ModelConfig{ModelID: "gemini-3.8-flash"})
	if got.GenerationConfig.ThinkingConfig == nil || got.GenerationConfig.ThinkingConfig.ThinkingBudget == nil {
		t.Fatal("gemini lost the budget the client stated")
	}
	if v := *got.GenerationConfig.ThinkingConfig.ThinkingBudget; v != 8192 {
		t.Fatalf("thinkingBudget = %d, want the client's 8192", v)
	}

	// A level-only client has no other way to express intent, so the level is
	// converted into the budget Gemini understands.
	level := &types.MessageRequest{
		Model: "claude-x", MaxTokens: 4096,
		OutputConfig: json.RawMessage(`{"effort":"high"}`),
		Messages:     firstTurn,
	}
	got = AnthropicToGemini(level, config.ModelConfig{ModelID: "gemini-3.8-flash"})
	if got.GenerationConfig.ThinkingConfig == nil || got.GenerationConfig.ThinkingConfig.ThinkingBudget == nil {
		t.Fatal("gemini lost the level the client stated")
	}
	if v, want := *got.GenerationConfig.ThinkingConfig.ThinkingBudget, core.BudgetForEffort("high"); v != want {
		t.Fatalf("thinkingBudget = %d, want %d for level high", v, want)
	}

	// Saying nothing about reasoning must leave the field absent rather than
	// inventing a budget.
	plain := &types.MessageRequest{Model: "claude-x", MaxTokens: 4096, Messages: firstTurn}
	if got := AnthropicToGemini(plain, config.ModelConfig{ModelID: "gemini-3.8-flash"}); got.GenerationConfig.ThinkingConfig != nil {
		t.Fatalf("an unstated budget produced thinkingConfig: %+v", got.GenerationConfig.ThinkingConfig)
	}
}

// Gemini publishes a budget range, and a request outside it asks for something
// the model cannot honour. The budget is clamped rather than forwarded.
func TestGeminiBudgetIsClampedToThePublishedRange(t *testing.T) {
	req := &types.MessageRequest{
		Model: "claude-x", MaxTokens: 4096,
		Thinking: json.RawMessage(`{"type":"enabled","budget_tokens":100000}`),
		Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"go"`)}},
	}
	got := AnthropicToGemini(req, config.ModelConfig{ModelID: "gemini-3.8-flash"})
	if got.GenerationConfig.ThinkingConfig == nil || got.GenerationConfig.ThinkingConfig.ThinkingBudget == nil {
		t.Fatal("gemini lost the budget")
	}
	if v := *got.GenerationConfig.ThinkingConfig.ThinkingBudget; v != 32768 {
		t.Fatalf("thinkingBudget = %d, want the published ceiling 32768", v)
	}

	// A level converts to a budget inside the range rather than above it.
	level := &types.MessageRequest{
		Model: "claude-x", MaxTokens: 4096,
		OutputConfig: json.RawMessage(`{"effort":"high"}`),
		Messages:     []types.Message{{Role: "user", Content: json.RawMessage(`"go"`)}},
	}
	got = AnthropicToGemini(level, config.ModelConfig{ModelID: "gemini-3.8-flash"})
	if v := *got.GenerationConfig.ThinkingConfig.ThinkingBudget; v != 32768 {
		t.Fatalf("thinkingBudget = %d, want 32768 for level high", v)
	}
}
