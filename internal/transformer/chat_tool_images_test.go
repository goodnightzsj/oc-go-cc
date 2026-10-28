package transformer

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestChatToolImagesPreserveParallelResultsAndContent(t *testing.T) {
	const first = `{"type":"tool_result","tool_use_id":"b","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGk="}}]}`
	const second = `{"type":"tool_result","tool_use_id":"a","content":[{"type":"text","text":"before"},{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}},{"type":"text","text":"between"},{"type":"image","source":{"type":"url","url":"https://example.invalid/b.png"}},{"type":"text","text":"after"}]}`
	for _, split := range []bool{false, true} {
		for _, vision := range []*bool{nil, config.BoolPtr(true)} {
			req := &types.MessageRequest{Messages: []types.Message{
				{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"a","name":"shot","input":{}},{"type":"tool_use","id":"b","name":"shot","input":{}}]`)},
			}}
			if split {
				req.Messages = append(req.Messages, types.Message{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"request prefix"},` + first + `]`)}, types.Message{Role: "user", Content: json.RawMessage(`[` + second + `,{"type":"text","text":"continue"}]`)})
			} else {
				req.Messages = append(req.Messages, types.Message{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"request prefix"},` + first + `,` + second + `,{"type":"text","text":"continue"}]`)})
			}
			before := mustJSONBytes(t, req)
			out, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "synthetic", Vision: vision})
			if err != nil {
				t.Fatalf("split=%t vision=%v: %v", split, vision, err)
			}
			if len(out.Messages) < 4 || out.Messages[0].Role != "assistant" || out.Messages[1].ToolCallID != "b" || out.Messages[2].ToolCallID != "a" {
				t.Fatalf("tool batch reordered: %+v", out.Messages)
			}
			for _, msg := range out.Messages[1:3] {
				var ref string
				if msg.Role != "tool" || json.Unmarshal(msg.Content, &ref) != nil || !strings.Contains(ref, msg.ToolCallID) || !strings.Contains(ref, "user message") {
					t.Fatalf("missing text-only tool association: %+v", msg)
				}
			}
			var ordered []string
			var projectedText strings.Builder
			for _, msg := range out.Messages[3:] {
				if msg.Role != "user" {
					t.Fatalf("non-user projection: %+v", msg)
				}
				var parts []types.ChatContentPart
				if err := json.Unmarshal(msg.Content, &parts); err != nil {
					t.Fatal(err)
				}
				for _, part := range parts {
					projectedText.WriteString(part.Text)
					if part.Type == "image_url" {
						ordered = append(ordered, part.ImageURL.URL)
					} else if !strings.Contains(part.Text, "tool_call_id") {
						ordered = append(ordered, part.Text)
					}
				}
			}
			for _, id := range []string{"a", "b"} {
				if strings.Count(projectedText.String(), `Tool result for tool_call_id "`+id+`":`) != 1 {
					t.Fatalf("projected result lost association with call %s", id)
				}
			}
			want := []string{"request prefix", "data:image/png;base64,aGk=", "before", "https://example.invalid/a.png", "between", "https://example.invalid/b.png", "after", "continue"}
			if !reflect.DeepEqual(ordered, want) {
				t.Fatalf("split=%t content lost/reordered/duplicated: got %q want %q", split, ordered, want)
			}
			if string(before) != string(mustJSONBytes(t, req)) {
				t.Fatal("source request mutated")
			}
		}
	}
}

func TestChatToolImageWaitsForTextOnlyParallelResult(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"a","name":"shot","input":{}},{"type":"tool_use","id":"b","name":"shot","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"a","content":[{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}}]}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"b","content":"done"}]`)},
	}}
	out, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "synthetic"})
	if err != nil || len(out.Messages) != 4 || out.Messages[1].ToolCallID != "a" || out.Messages[2].ToolCallID != "b" || out.Messages[2].ContentText() != "done" || out.Messages[3].Role != "user" {
		t.Fatalf("image interrupted or lost final text-only tool result: %+v err=%v", out, err)
	}
}

func TestChatToolImagesRejectIncompleteToolBatch(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{
		{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"a","name":"shot","input":{}},{"type":"tool_use","id":"b","name":"shot","input":{}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"a","content":[{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}}]}]`)},
	}}
	for _, completedTurn := range []bool{false, true} {
		if completedTurn {
			req.Messages = append(req.Messages, types.Message{Role: "assistant", Content: json.RawMessage(`"not complete"`)})
		}
		if _, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "synthetic"}); err == nil || !strings.Contains(err.Error(), "tool responses") {
			t.Fatalf("projected images before outstanding tool responses: %v", err)
		}
	}
}

func TestChatToolImagesValidateSourcesAndVision(t *testing.T) {
	for _, source := range []string{`null`, `{"type":"url"}`, `{"type":"base64","media_type":"image/png"}`, `{"type":"unsupported"}`} {
		req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"a","content":[{"type":"image","source":` + source + `}]}]`)}}}
		if _, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "synthetic"}); err == nil {
			t.Fatalf("accepted invalid image source %s", source)
		}
	}
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"a","content":[{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}}]}]`)}}}
	if _, err := AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "synthetic", Vision: config.BoolPtr(false)}); err == nil || !strings.Contains(err.Error(), "vision-capable") {
		t.Fatalf("non-vision tool image silently replaced: %v", err)
	}
}

func TestChatToolImagesKeepCacheBreakpointsOnTheirParts(t *testing.T) {
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"a","content":[{"type":"text","text":"before","cache_control":{"type":"ephemeral"}},{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"},"cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"text","text":"after"}]}]`)}}}
	for _, provider := range []string{"openrouter", "cline-pass"} {
		out, err := AnthropicToChatCompletion(req, config.ModelConfig{Provider: provider, ModelID: "anthropic/claude-sonnet-4.6"})
		if err != nil {
			t.Fatal(err)
		}
		var parts []types.ChatContentPart
		if err := json.Unmarshal(out.Messages[1].Content, &parts); err != nil {
			t.Fatal(err)
		}
		for _, part := range parts {
			wantCache := provider == "openrouter" && (part.Type == "image_url" || part.Text == "before")
			if (part.CacheControl != nil) != wantCache {
				t.Fatalf("provider=%s cache moved or lost: %+v", provider, part)
			}
			if wantCache && part.Type == "image_url" && part.CacheControl.TTL != "1h" {
				t.Fatal("image TTL lost")
			}
		}
	}
}
