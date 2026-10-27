package transformer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestResponsesToolOutputPreservesImagesAndCallOrder(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"a","name":"shot","input":{}},{"type":"tool_use","id":"b","name":"shot","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"b","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGk="}}]},{"type":"tool_result","tool_use_id":"a","content":[{"type":"text","text":"before"},{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}},{"type":"text","text":"after"}]}]`)},
	}}
	wire, err := AnthropicToResponses(req, config.ModelConfig{ModelID: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Input []struct {
			CallID string          `json:"call_id"`
			Output json.RawMessage `json:"output"`
		} `json:"input"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Input) != 4 || body.Input[2].CallID != "b" || body.Input[3].CallID != "a" {
		t.Fatalf("call order lost: %s", encoded)
	}
	var parts []map[string]string
	if err := json.Unmarshal(body.Input[2].Output, &parts); err != nil || len(parts) != 1 || parts[0]["type"] != "input_image" || parts[0]["image_url"] != "data:image/png;base64,aGk=" {
		t.Fatalf("image-only result lost: %s", encoded)
	}
	if err := json.Unmarshal(body.Input[3].Output, &parts); err != nil || len(parts) != 3 || parts[0]["text"] != "before" || parts[1]["image_url"] != "https://example.invalid/a.png" || parts[2]["text"] != "after" {
		t.Fatalf("mixed result reordered: %s", encoded)
	}
}

func TestToolOutputUnsupportedContentIsNotDiscarded(t *testing.T) {
	for _, content := range []string{
		`[{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}}]`,
		`[{"type":"text","text":"prefix"},{"type":"tool_reference","tool_name":"lookup"}]`,
		`{"invalid":"shape"}`,
	} {
		req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"a","content":` + content + `}]`)}}}
		if _, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "synthetic"}); err == nil {
			t.Errorf("Chat silently lost %s", content)
		}
		model := config.ModelConfig{ModelID: "synthetic", Vision: config.BoolPtr(false)}
		if _, err := AnthropicToResponses(req, model); err == nil {
			t.Errorf("Responses silently accepted unsupported %s", content)
		}
	}
}

func TestToolOutputTextCompatibility(t *testing.T) {
	for _, content := range []string{`"hello"`, `[{"type":"text","text":"hel"},{"type":"text","text":"lo"}]`} {
		req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"a","content":` + content + `}]`)}}}
		wire, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "synthetic"})
		if err != nil || len(wire.Messages) != 1 || string(wire.Messages[0].Content) != `"hello"` {
			t.Fatalf("text compatibility changed: %+v %v", wire, err)
		}
		responses, err := AnthropicToResponses(req, config.ModelConfig{ModelID: "synthetic"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(responses.Input[0].Output), "hel") {
			t.Fatal("text lost")
		}
	}
}

func TestResponsesInboundToolImagesReachSelectedWire(t *testing.T) {
	req, err := ResponsesToMessageRequest([]byte(`{"model":"m","input":[{"type":"function_call","call_id":"c","name":"shot","arguments":"{}"},{"type":"function_call_output","call_id":"c","output":[{"type":"input_text","text":"before"},{"type":"input_image","image_url":"https://example.invalid/a.png"},{"type":"input_text","text":"after"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := AnthropicToResponses(req, config.ModelConfig{ModelID: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	var parts []map[string]string
	if err := json.Unmarshal(out.Input[1].Output, &parts); err != nil || len(parts) != 3 || parts[0]["text"] != "before" || parts[1]["image_url"] != "https://example.invalid/a.png" || parts[2]["text"] != "after" {
		t.Fatalf("tool image round trip lost: %+v err=%v", out, err)
	}
	if _, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "synthetic"}); err == nil {
		t.Fatal("Chat silently accepted image-bearing tool output")
	}
}
