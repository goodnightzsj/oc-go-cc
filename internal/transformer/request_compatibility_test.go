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

func TestRequestRejectsDroppedToolResultContent(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[
		{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"screenshot"},{"type":"image","source":{"type":"url","url":"https://example.test/one.png"}}]}
	]`)}}}
	if _, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "vision-model", Vision: true}); err == nil {
		t.Fatal("tool result image must not silently disappear")
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
