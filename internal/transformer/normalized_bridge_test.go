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

	responsesReq := AnthropicToResponses(req, config.ModelConfig{ModelID: "gpt-5"})

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
