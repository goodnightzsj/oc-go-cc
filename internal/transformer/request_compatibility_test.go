package transformer

import (
	"encoding/json"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestRequestPreservesImageSourcesAndOrder(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[
		{"type":"text","text":"first"},
		{"type":"image","source":{"type":"url","url":"https://example.test/one.png"}},
		{"type":"text","text":"second"},
		{"type":"image","source":{"type":"base64","media_type":"image/png","data":"YWJj"}},
		{"type":"text","text":"last"}
	]`)}}}
	got, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "vision-model", Vision: true})
	if err != nil {
		t.Fatal(err)
	}
	var parts []types.ChatContentPart
	if len(got.Messages) != 1 {
		t.Fatalf("messages = %d", len(got.Messages))
	}
	if err := json.Unmarshal(got.Messages[0].Content, &parts); err != nil {
		t.Fatal(err)
	}
	if len(parts) != 5 || parts[0].Text != "first" || parts[2].Text != "second" || parts[4].Text != "last" {
		t.Fatalf("content order changed: %s", got.Messages[0].Content)
	}
	if parts[1].ImageURL == nil || parts[1].ImageURL.URL != "https://example.test/one.png" ||
		parts[3].ImageURL == nil || parts[3].ImageURL.URL != "data:image/png;base64,YWJj" {
		t.Fatalf("image sources changed: %s", got.Messages[0].Content)
	}
}

// A tool_result may carry parts Chat Completions has no place for - an image,
// or Claude Code's tool_reference. They are dropped and the text parts are
// kept, matching the reference CommandCode proxy. The request succeeds; the
// tool message simply carries less than the client sent.
//
// This replaced a check that failed the whole request instead. The trade is
// deliberate, so this pins the drop: if a part ever stops being dropped, or the
// request starts failing again, that is a behaviour change worth noticing.
func TestRequestDropsUnrepresentableToolResultParts(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[
		{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"screenshot"},{"type":"image","source":{"type":"url","url":"https://example.test/one.png"}}]}
	]`)}}}
	got, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "vision-model", Vision: true})
	if err != nil {
		t.Fatalf("an unrepresentable part must not fail the request: %v", err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "tool" {
		t.Fatalf("expected one tool message, got %+v", got.Messages)
	}
	var text string
	if err := json.Unmarshal(got.Messages[0].Content, &text); err != nil {
		t.Fatalf("tool content is not text: %s", got.Messages[0].Content)
	}
	if text != "screenshot" {
		t.Errorf("the representable part must survive; got %q", text)
	}
}

// A tool_result carrying only unrepresentable parts becomes an empty tool
// message. This is the ToolSearch case: the model proceeds without the tool's
// content rather than the chain failing.
func TestRequestKeepsEmptyToolMessageForToolReference(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[
		{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"tool_reference","tool_name":"Monitor"}]}
	]`)}}}
	got, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "deepseek-v4.1-flash"})
	if err != nil {
		t.Fatalf("a tool_reference must not fail the request: %v", err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "tool" || got.Messages[0].ToolCallID != "call_1" {
		t.Fatalf("expected one tool message keyed to call_1, got %+v", got.Messages)
	}
	var text string
	if err := json.Unmarshal(got.Messages[0].Content, &text); err != nil {
		t.Fatalf("tool content is not text: %s", got.Messages[0].Content)
	}
	if text != "" {
		t.Errorf("expected an empty tool message, got %q", text)
	}
}

func TestRequestCarriesToolChoiceParallelAndEffort(t *testing.T) {
	var req types.MessageRequest
	if err := json.Unmarshal([]byte(`{"model":"example","messages":[{"role":"user","content":"hello"}],"tool_choice":{"type":"tool","name":"read_file","disable_parallel_tool_use":true},"output_config":{"effort":"high"}}`), &req); err != nil {
		t.Fatal(err)
	}
	got, err := AnthropicToChatCompletion(&req, config.ModelConfig{ModelID: "example"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		ToolChoice struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tool_choice"`
		Parallel *bool  `json:"parallel_tool_calls"`
		Effort   string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.ToolChoice.Type != "function" || wire.ToolChoice.Function.Name != "read_file" || wire.Parallel == nil || *wire.Parallel || wire.Effort != "high" {
		t.Fatalf("request constraints lost: %s", raw)
	}
}
