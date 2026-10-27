package transformer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestOpenRouterCacheBreakpointsStayOnTheirContent(t *testing.T) {
	const request = `{"system":[{"type":"text","text":"prefix","cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"text","text":"dynamic"}],"messages":[{"role":"user","content":[{"type":"text","text":"cached","cache_control":{"type":"ephemeral"}},{"type":"text","text":"tail"}]},{"role":"assistant","content":[{"type":"text","text":"answer","cache_control":{"type":"ephemeral"}}]}]}`
	for _, model := range []string{"anthropic/claude-sonnet-4.6", "google/gemini-2.5-pro"} {
		var req types.MessageRequest
		if err := json.Unmarshal([]byte(request), &req); err != nil {
			t.Fatal(err)
		}
		before, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		for _, provider := range []string{"openrouter", "opencode-zen"} {
			out, err := AnthropicToChatCompletion(&req, config.ModelConfig{Provider: provider, ModelID: model})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			if provider != "openrouter" {
				if strings.Contains(string(raw), "cache_control") {
					t.Fatalf("cache extension leaked to %s: %s", provider, raw)
				}
				continue
			}
			for i, msg := range out.Messages {
				if msg.CacheControl != nil {
					t.Fatal("content breakpoint moved onto whole message")
				}
				var parts []types.ChatContentPart
				if err := json.Unmarshal(msg.Content, &parts); err != nil {
					t.Fatal(err)
				}
				if parts[0].CacheControl == nil || parts[0].CacheControl.Type != "ephemeral" {
					t.Fatalf("cache breakpoint lost: %s", raw)
				}
				if i < 2 && (len(parts) != 2 || parts[1].CacheControl != nil) {
					t.Fatalf("dynamic tail cached: %s", raw)
				}
				if i == 0 && parts[0].CacheControl.TTL != "1h" {
					t.Fatalf("TTL lost: %s", raw)
				}
			}
		}
		after, err := json.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatal("source mutated across fallback targets")
		}
	}
}

func TestResponsesCacheKeyReachesResponsesWireOnly(t *testing.T) {
	req, err := ResponsesToMessageRequest([]byte(`{"model":"synthetic","input":"hello","prompt_cache_key":"synthetic-affinity"}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := AnthropicToResponses(req, config.ModelConfig{ModelID: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"prompt_cache_key":"synthetic-affinity"`) {
		t.Fatalf("affinity lost: %s", raw)
	}
	if strings.Contains(string(raw), "previous_response_id") {
		t.Fatal("cache affinity became remote conversation state")
	}
	anthropic, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(anthropic), "prompt_cache_key") {
		t.Fatal("Responses-only field leaked into Messages")
	}
}

func TestCacheBreakpointsOnMessageSystemAndNonVisionPlaceholder(t *testing.T) {
	for _, role := range []string{"system", "user"} {
		req := &types.MessageRequest{Messages: []types.Message{{Role: role, Content: json.RawMessage(`[{"type":"text","text":"cached","cache_control":{"type":"ephemeral"}},{"type":"text","text":"tail"}]`)}}}
		if role == "user" {
			req.Messages[0].Content = json.RawMessage(`[{"type":"text","text":"cached","cache_control":{"type":"ephemeral"}},{"type":"image","source":{"type":"url","url":"https://example.invalid/image.png"}}]`)
		}
		model := config.ModelConfig{Provider: "openrouter", ModelID: "anthropic/claude-sonnet-4.6", Vision: config.BoolPtr(false)}
		out, err := AnthropicToChatCompletion(req, model)
		if err != nil {
			t.Fatal(err)
		}
		var parts []types.ChatContentPart
		if err := json.Unmarshal(out.Messages[0].Content, &parts); err != nil || len(parts) != 2 || parts[0].CacheControl == nil || parts[1].CacheControl != nil {
			t.Fatalf("role=%s content=%s err=%v", role, out.Messages[0].Content, err)
		}
		if role == "user" && !strings.Contains(parts[1].Text, "[Image]") {
			t.Fatal("legacy non-vision placeholder lost when text is cached")
		}
	}
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`[{"type":"image","source":{"type":"url","url":"https://example.invalid/image.png"},"cache_control":{"type":"ephemeral"}}]`)}}}
	if _, err := AnthropicToChatCompletion(req, config.ModelConfig{Provider: "openrouter", ModelID: "claude-sonnet-4.6", Vision: config.BoolPtr(false)}); err == nil {
		t.Fatal("image cache breakpoint silently moved onto a placeholder")
	}
}
