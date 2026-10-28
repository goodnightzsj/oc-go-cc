package transformer

import (
	"encoding/json"
	"strings"
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
	got, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "vision-model", Vision: config.BoolPtr(true)})
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

func TestRequestProjectsToolResultImages(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[
		{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"screenshot"},{"type":"image","source":{"type":"url","url":"https://example.test/one.png"}}]}
	]`)}}}
	out, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "vision-model", Vision: config.BoolPtr(true)})
	if err != nil || len(out.Messages) != 2 || out.Messages[0].Role != "tool" || out.Messages[0].ToolCallID != "call_1" || out.Messages[1].Role != "user" {
		t.Fatalf("tool result/image association lost: %+v err=%v", out, err)
	}
	var parts []types.ChatContentPart
	if err := json.Unmarshal(out.Messages[1].Content, &parts); err != nil || len(parts) != 4 || parts[1].Text != "screenshot" || parts[2].ImageURL == nil || parts[2].ImageURL.URL != "https://example.test/one.png" {
		t.Fatalf("tool image missing from projected content: %s err=%v", out.Messages[1].Content, err)
	}
}

func TestRequestRejectsToolReferenceWithoutAnEquivalent(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[
		{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"tool_reference","tool_name":"Monitor"}]}
	]`)}}}
	_, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "deepseek-v4.1-flash"})
	if err == nil || !strings.Contains(err.Error(), `tool_result content "tool_reference"`) {
		t.Fatalf("expected an explicit unsupported tool_reference error, got %v", err)
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

// An unstated vision capability must forward the image, not replace it.
//
// The alternative is the failure this replaced: the model was sent the literal
// text "[Image]", answered confidently about seven characters, and nothing
// reported that the picture never left. A 400 from the platform is a far better
// outcome than a plausible answer about nothing.
func TestUnstatedVisionForwardsTheImageRatherThanAPlaceholder(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[
		{"type":"text","text":"what is this"},
		{"type":"image","source":{"type":"url","url":"https://example.test/one.png"}}
	]`)}}}
	got, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "unknown-model"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(got.Messages))
	}
	var parts []types.ChatContentPart
	if err := json.Unmarshal(got.Messages[0].Content, &parts); err != nil {
		t.Fatalf("content is not a multimodal array, so the image was dropped: %s", got.Messages[0].Content)
	}
	found := false
	for _, p := range parts {
		if p.Type == "image_url" && p.ImageURL != nil && p.ImageURL.URL == "https://example.test/one.png" {
			found = true
		}
	}
	if !found {
		t.Errorf("image not forwarded: %+v", parts)
	}
}

// A model known to refuse images still gets the placeholder, which keeps the
// rest of the conversation usable on a platform that cannot see.
func TestKnownTextOnlyModelStillGetsThePlaceholder(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[
		{"type":"text","text":"what is this"},
		{"type":"image","source":{"type":"url","url":"https://example.test/one.png"}}
	]`)}}}
	got, err := AnthropicToChatCompletion(req, config.ModelConfig{
		ModelID: "text-only", Vision: config.BoolPtr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	content := string(got.Messages[0].Content)
	if !strings.Contains(content, "[Image]") {
		t.Errorf("expected the placeholder for a known text-only model, got %s", content)
	}
}
