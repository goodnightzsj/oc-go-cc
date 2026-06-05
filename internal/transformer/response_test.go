package transformer

import (
	"encoding/json"
	"strings"
	"testing"

	"oc-go-cc/pkg/types"
)

func TestTransformResponsePreservesReasoningContent(t *testing.T) {
	transformer := NewResponseTransformer()

	reasoning := "Let me think about this step by step"
	resp := &types.ChatCompletionResponse{
		ID:      "chatcmpl_123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "kimi-k2.6",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:             "assistant",
					Content:          "The answer is 42.",
					ReasoningContent: &reasoning,
				},
				FinishReason: "stop",
			},
		},
		Usage: usageInfoPtr(types.UsageInfo{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		}),
	}

	anthropicResp, err := transformer.TransformResponse(resp, "kimi-k2.6")
	if err != nil {
		t.Fatalf("TransformResponse() error = %v", err)
	}

	if got, want := len(anthropicResp.Content), 2; got != want {
		t.Fatalf("len(Content) = %d, want %d", got, want)
	}

	if got, want := anthropicResp.Content[0].Type, "thinking"; got != want {
		t.Fatalf("Content[0].Type = %q, want %q", got, want)
	}
	if got, want := anthropicResp.Content[0].Thinking, reasoning; got != want {
		t.Fatalf("Content[0].Thinking = %q, want %q", got, want)
	}

	if got, want := anthropicResp.Content[1].Type, "text"; got != want {
		t.Fatalf("Content[1].Type = %q, want %q", got, want)
	}
	if got, want := anthropicResp.Content[1].Text, "The answer is 42."; got != want {
		t.Fatalf("Content[1].Text = %q, want %q", got, want)
	}
}

func TestTransformResponsePreservesReasoningContentWithToolCalls(t *testing.T) {
	transformer := NewResponseTransformer()

	reasoning := "I need to call a tool to get the weather"
	resp := &types.ChatCompletionResponse{
		ID:      "chatcmpl_456",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "kimi-k2.6",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:             "assistant",
					Content:          "",
					ReasoningContent: &reasoning,
					ToolCalls: []types.ToolCall{
						{
							ID:   "call_123",
							Type: "function",
							Function: types.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"city":"Kigali"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: usageInfoPtr(types.UsageInfo{
			PromptTokens:     20,
			CompletionTokens: 15,
			TotalTokens:      35,
		}),
	}

	anthropicResp, err := transformer.TransformResponse(resp, "kimi-k2.6")
	if err != nil {
		t.Fatalf("TransformResponse() error = %v", err)
	}

	if got, want := len(anthropicResp.Content), 2; got != want {
		t.Fatalf("len(Content) = %d, want %d", got, want)
	}

	if got, want := anthropicResp.Content[0].Type, "thinking"; got != want {
		t.Fatalf("Content[0].Type = %q, want %q", got, want)
	}
	if got, want := anthropicResp.Content[0].Thinking, reasoning; got != want {
		t.Fatalf("Content[0].Thinking = %q, want %q", got, want)
	}

	if got, want := anthropicResp.Content[1].Type, "tool_use"; got != want {
		t.Fatalf("Content[1].Type = %q, want %q", got, want)
	}
	if got, want := anthropicResp.Content[1].Name, "get_weather"; got != want {
		t.Fatalf("Content[1].Name = %q, want %q", got, want)
	}

	if got, want := anthropicResp.StopReason, "tool_use"; got != want {
		t.Fatalf("StopReason = %q, want %q", got, want)
	}
}

func TestTransformResponseOmitsEmptyReasoningContent(t *testing.T) {
	transformer := NewResponseTransformer()

	emptyReasoning := ""
	resp := &types.ChatCompletionResponse{
		ID:      "chatcmpl_789",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "kimi-k2.6",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:             "assistant",
					Content:          "Hello there.",
					ReasoningContent: &emptyReasoning,
				},
				FinishReason: "stop",
			},
		},
		Usage: usageInfoPtr(types.UsageInfo{
			PromptTokens:     5,
			CompletionTokens: 2,
			TotalTokens:      7,
		}),
	}

	anthropicResp, err := transformer.TransformResponse(resp, "kimi-k2.6")
	if err != nil {
		t.Fatalf("TransformResponse() error = %v", err)
	}

	if got, want := len(anthropicResp.Content), 1; got != want {
		t.Fatalf("len(Content) = %d, want %d", got, want)
	}

	if got, want := anthropicResp.Content[0].Type, "text"; got != want {
		t.Fatalf("Content[0].Type = %q, want %q", got, want)
	}
}

func TestTransformResponseNoReasoningContent(t *testing.T) {
	transformer := NewResponseTransformer()

	resp := &types.ChatCompletionResponse{
		ID:      "chatcmpl_abc",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "kimi-k2.6",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:    "assistant",
					Content: "Just a plain response.",
				},
				FinishReason: "stop",
			},
		},
		Usage: usageInfoPtr(types.UsageInfo{
			PromptTokens:     3,
			CompletionTokens: 4,
			TotalTokens:      7,
		}),
	}

	anthropicResp, err := transformer.TransformResponse(resp, "kimi-k2.6")
	if err != nil {
		t.Fatalf("TransformResponse() error = %v", err)
	}

	if got, want := len(anthropicResp.Content), 1; got != want {
		t.Fatalf("len(Content) = %d, want %d", got, want)
	}

	if got, want := anthropicResp.Content[0].Type, "text"; got != want {
		t.Fatalf("Content[0].Type = %q, want %q", got, want)
	}
}

func TestTransformResponseWithCacheTokens(t *testing.T) {
	transformer := NewResponseTransformer()

	openaiResp := &types.ChatCompletionResponse{
		ID:     "chatcmpl-123",
		Object: "chat.completion",
		Model:  "kimi-k2.6",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:    "assistant",
					Content: "Hello, world!",
				},
				FinishReason: "stop",
			},
		},
		Usage: usageInfoPtr(types.UsageInfo{
			PromptTokens:          100,
			CompletionTokens:      50,
			TotalTokens:           150,
			PromptCacheHitTokens:  80,
			PromptCacheMissTokens: 20,
		}),
	}

	anthropicResp, err := transformer.TransformResponse(openaiResp, "claude-3-sonnet")
	if err != nil {
		t.Fatalf("TransformResponse() error = %v", err)
	}

	// Per Anthropic spec, input_tokens excludes cache reads AND cache
	// creations. Upstream prompt_tokens=100 split as 80 hit + 20 miss
	// means everything was accounted for by the cache → input_tokens = 0.
	usage := requireUsage(t, anthropicResp)
	if got, want := usage.InputTokens, 0; got != want {
		t.Errorf("Usage.InputTokens = %d, want %d", got, want)
	}
	if got, want := usage.OutputTokens, 50; got != want {
		t.Errorf("Usage.OutputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheReadInputTokens, 80; got != want {
		t.Errorf("Usage.CacheReadInputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheCreationInputTokens, 20; got != want {
		t.Errorf("Usage.CacheCreationInputTokens = %d, want %d", got, want)
	}
}

// TestTransformResponseWithPartialCacheTokens covers the case where the
// upstream's hit + miss don't fully account for prompt_tokens (e.g., a
// portion of the prompt is below the prefix-cache minimum and reported as
// neither cached nor newly cached). The leftover should map to input_tokens.
func TestTransformResponseWithPartialCacheTokens(t *testing.T) {
	transformer := NewResponseTransformer()

	openaiResp := &types.ChatCompletionResponse{
		ID:     "chatcmpl-789",
		Object: "chat.completion",
		Model:  "deepseek-v4-pro",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:    "assistant",
					Content: "ok",
				},
				FinishReason: "stop",
			},
		},
		Usage: usageInfoPtr(types.UsageInfo{
			PromptTokens:          100,
			CompletionTokens:      5,
			TotalTokens:           105,
			PromptCacheHitTokens:  60,
			PromptCacheMissTokens: 30,
			// 100 - 60 - 30 = 10 tokens are neither cached nor newly cached.
		}),
	}

	anthropicResp, err := transformer.TransformResponse(openaiResp, "claude-3-sonnet")
	if err != nil {
		t.Fatalf("TransformResponse() error = %v", err)
	}

	usage := requireUsage(t, anthropicResp)
	if got, want := usage.InputTokens, 10; got != want {
		t.Errorf("Usage.InputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheReadInputTokens, 60; got != want {
		t.Errorf("Usage.CacheReadInputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheCreationInputTokens, 30; got != want {
		t.Errorf("Usage.CacheCreationInputTokens = %d, want %d", got, want)
	}
}

// TestTransformResponseCacheExceedsPromptTokens covers the defensive edge
// case where upstream reports cache_hit + cache_miss > prompt_tokens.
// The nonNegative guard must clamp input_tokens to 0 instead of going negative.
func TestTransformResponseCacheExceedsPromptTokens(t *testing.T) {
	transformer := NewResponseTransformer()

	openaiResp := &types.ChatCompletionResponse{
		ID:     "chatcmpl-overflow",
		Object: "chat.completion",
		Model:  "deepseek-v4-pro",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:    "assistant",
					Content: "ok",
				},
				FinishReason: "stop",
			},
		},
		Usage: usageInfoPtr(types.UsageInfo{
			PromptTokens:          50,
			CompletionTokens:      5,
			TotalTokens:           55,
			PromptCacheHitTokens:  40,
			PromptCacheMissTokens: 20,
			// 50 - 40 - 20 = -10, clamped to 0
		}),
	}

	anthropicResp, err := transformer.TransformResponse(openaiResp, "claude-3-sonnet")
	if err != nil {
		t.Fatalf("TransformResponse() error = %v", err)
	}

	usage := requireUsage(t, anthropicResp)
	if got, want := usage.InputTokens, 0; got != want {
		t.Errorf("Usage.InputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheReadInputTokens, 40; got != want {
		t.Errorf("Usage.CacheReadInputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheCreationInputTokens, 20; got != want {
		t.Errorf("Usage.CacheCreationInputTokens = %d, want %d", got, want)
	}
}

func TestTransformResponseWithoutCacheTokens(t *testing.T) {
	transformer := NewResponseTransformer()

	openaiResp := &types.ChatCompletionResponse{
		ID:     "chatcmpl-456",
		Object: "chat.completion",
		Model:  "glm-5",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:    "assistant",
					Content: "No cache here",
				},
				FinishReason: "stop",
			},
		},
		Usage: usageInfoPtr(types.UsageInfo{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		}),
	}

	anthropicResp, err := transformer.TransformResponse(openaiResp, "claude-3-haiku")
	if err != nil {
		t.Fatalf("TransformResponse() error = %v", err)
	}

	usage := requireUsage(t, anthropicResp)
	if got, want := usage.InputTokens, 10; got != want {
		t.Errorf("Usage.InputTokens = %d, want %d", got, want)
	}
	if got, want := usage.OutputTokens, 5; got != want {
		t.Errorf("Usage.OutputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheReadInputTokens, 0; got != want {
		t.Errorf("Usage.CacheReadInputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheCreationInputTokens, 0; got != want {
		t.Errorf("Usage.CacheCreationInputTokens = %d, want %d", got, want)
	}
}

func TestTransformResponseOmitsMissingUsage(t *testing.T) {
	transformer := NewResponseTransformer()

	openaiResp := &types.ChatCompletionResponse{
		ID:     "chatcmpl-no-usage",
		Object: "chat.completion",
		Model:  "qwen3.6-plus",
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:    "assistant",
					Content: "No usage was reported.",
				},
				FinishReason: "stop",
			},
		},
	}

	anthropicResp, err := transformer.TransformResponse(openaiResp, "claude-3-haiku")
	if err != nil {
		t.Fatalf("TransformResponse() error = %v", err)
	}
	if anthropicResp.Usage != nil {
		t.Fatalf("Usage = %+v, want nil when upstream omits usage", anthropicResp.Usage)
	}

	body, err := json.Marshal(anthropicResp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(body), `"usage"`) {
		t.Fatalf("serialized response contains usage despite missing upstream usage: %s", body)
	}
}

func TestTransformResponsesResponseOmitsMissingUsage(t *testing.T) {
	transformer := NewResponseTransformer()

	responsesResp := &types.ResponsesResponse{
		ID:    "resp-no-usage",
		Model: "gpt-5",
		Output: []types.ResponsesOutput{
			{
				Type: "message",
				Content: []types.ResponsesContent{
					{Type: "output_text", Text: "No usage was reported."},
				},
			},
		},
	}

	anthropicResp, err := transformer.TransformResponsesResponse(responsesResp, "claude-3-haiku")
	if err != nil {
		t.Fatalf("TransformResponsesResponse() error = %v", err)
	}
	if anthropicResp.Usage != nil {
		t.Fatalf("Usage = %+v, want nil when upstream omits usage", anthropicResp.Usage)
	}
}
