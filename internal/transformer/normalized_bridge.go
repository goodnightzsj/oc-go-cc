package transformer

import (
	"encoding/json"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

// ── Request-side: NormalizedRequest → wire format ─────────────────────

// AnthropicForModel returns a copy of the request addressed to the given model
// with the stream flag set explicitly. Providers on the Anthropic wire format
// pass the caller's request through unchanged apart from those two fields.
func AnthropicForModel(req *types.MessageRequest, model config.ModelConfig, stream bool) *types.MessageRequest {
	out := *req
	out.Model = model.ModelID
	if stream {
		out.Stream = &stream
	} else {
		out.Stream = nil
	}
	return &out
}

// AnthropicToResponses converts an Anthropic request to a ResponsesRequest.
func AnthropicToResponses(anthropicReq *types.MessageRequest, model config.ModelConfig) *types.ResponsesRequest {
	req := core.NormalizeRequest(anthropicReq)
	responsesReq := &types.ResponsesRequest{
		Model: model.ModelID,
	}

	// System prompt becomes a "developer" role input.
	if req.SystemPrompt != "" {
		responsesReq.Input = append(responsesReq.Input, types.ResponsesInput{
			Role:    "developer",
			Content: rawJSONString(req.SystemPrompt),
		})
	}

	// Convert messages.
	for _, msg := range req.Messages {
		input := types.ResponsesInput{Role: msg.Role}
		content := msg.Content

		// For assistant messages with tool calls, serialize as text.
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				content += "[Tool: " + tc.Name + "(" + tc.Arguments + ")]"
			}
		}

		if content != "" {
			input.Content = rawJSONString(content)
		}
		responsesReq.Input = append(responsesReq.Input, input)
	}

	// Convert tools.
	for _, tool := range req.Tools {
		responsesReq.Tools = append(responsesReq.Tools, types.ResponsesTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
		})
	}

	return responsesReq
}

// AnthropicToGemini converts an Anthropic request to a GeminiRequest.
func AnthropicToGemini(anthropicReq *types.MessageRequest, model config.ModelConfig) *types.GeminiRequest {
	req := core.NormalizeRequest(anthropicReq)
	geminiReq := &types.GeminiRequest{
		GenerationConfig: &types.GeminiGenerationConfig{
			MaxOutputTokens: req.MaxTokens,
		},
	}

	if req.Temperature != nil {
		geminiReq.GenerationConfig.Temperature = *req.Temperature
	}

	// System prompt is prepended as a user message (Gemini has no system role).
	var contents []types.GeminiContent
	if req.SystemPrompt != "" {
		contents = append(contents, types.GeminiContent{
			Role:  "user",
			Parts: []types.GeminiPart{{Text: req.SystemPrompt}},
		})
	}

	// Convert messages.
	for _, msg := range req.Messages {
		gc := types.GeminiContent{Role: msg.Role}
		gc.Parts = append(gc.Parts, types.GeminiPart{Text: msg.Content})
		contents = append(contents, gc)
	}

	geminiReq.Contents = contents

	// Convert tools.
	if len(req.Tools) > 0 {
		var functions []types.GeminiFunctionDeclaration
		for _, tool := range req.Tools {
			functions = append(functions, types.GeminiFunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			})
		}
		geminiReq.Tools = []types.GeminiTool{
			{FunctionDeclarations: functions},
		}
	}

	return geminiReq
}

// ── Response-side: wire format → NormalizedResponse ───────────────────

// OpenAIResponseToNormalized converts an OpenAI ChatCompletionResponse to NormalizedResponse.
func OpenAIResponseToNormalized(openaiResp *types.ChatCompletionResponse, modelID string) *core.NormalizedResponse {
	nr := &core.NormalizedResponse{
		ID:    openaiResp.ID,
		Model: modelID,
	}

	for _, choice := range openaiResp.Choices {
		msg := choice.Message

		nm := core.NormalizedMessage{Role: msg.Role}

		// Extract text content.
		if msg.Content != nil {
			nm.Content = msg.ContentText()
		}

		// Extract reasoning content (pointer field).
		if msg.ReasoningContent != nil {
			nm.Thinking = *msg.ReasoningContent
		}

		// Extract tool calls.
		for _, tc := range msg.ToolCalls {
			nm.ToolCalls = append(nm.ToolCalls, core.NormalizedToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}

		nr.Messages = append(nr.Messages, nm)

		// Map finish reason.
		switch choice.FinishReason {
		case "stop":
			nr.StopReason = "end_turn"
		case "length":
			nr.StopReason = "max_tokens"
		case "tool_calls":
			nr.StopReason = "tool_use"
		default:
			nr.StopReason = "end_turn"
		}
	}

	// Map usage. UsageInfo is a value type; check if it was populated.
	if openaiResp.Usage.PromptTokens > 0 || openaiResp.Usage.CompletionTokens > 0 {
		// OpenAI-standard cached_tokens counts as cache read; DeepSeek-style
		// hit/miss fields are read separately.
		cacheRead := openaiResp.Usage.PromptCacheHitTokens
		if openaiResp.Usage.PromptTokensDetails != nil {
			cacheRead += openaiResp.Usage.PromptTokensDetails.CachedTokens
		}
		nr.Usage = core.NormalizedUsage{
			InputTokens:         openaiResp.Usage.PromptTokens,
			OutputTokens:        openaiResp.Usage.CompletionTokens,
			CacheReadTokens:     cacheRead,
			CacheCreationTokens: openaiResp.Usage.PromptCacheMissTokens,
		}
	}

	return nr
}

// ResponsesToNormalized converts an OpenAI ResponsesResponse to NormalizedResponse.
func ResponsesToNormalized(responsesResp *types.ResponsesResponse, modelID string) *core.NormalizedResponse {
	nr := &core.NormalizedResponse{
		ID:    responsesResp.ID,
		Model: modelID,
	}

	for _, output := range responsesResp.Output {
		switch output.Type {
		case "message":
			nm := core.NormalizedMessage{Role: output.Role}
			for _, c := range output.Content {
				if c.Type == "output_text" {
					nm.Content += c.Text
				}
			}
			nr.Messages = append(nr.Messages, nm)
		case "function_call":
			nm := core.NormalizedMessage{
				Role: "assistant",
				ToolCalls: []core.NormalizedToolCall{
					{
						ID:        output.CallID,
						Name:      output.Name,
						Arguments: output.Arguments,
					},
				},
			}
			nr.Messages = append(nr.Messages, nm)
		}
	}

	nr.StopReason = "end_turn"

	nr.Usage = core.NormalizedUsage{
		InputTokens:  responsesResp.Usage.InputTokens,
		OutputTokens: responsesResp.Usage.OutputTokens,
	}

	return nr
}

// GeminiToNormalized converts a GeminiResponse to NormalizedResponse.
func GeminiToNormalized(geminiResp *types.GeminiResponse, modelID string) *core.NormalizedResponse {
	nr := &core.NormalizedResponse{
		Model: modelID,
	}

	if len(geminiResp.Candidates) > 0 {
		candidate := geminiResp.Candidates[0]
		nm := core.NormalizedMessage{Role: candidate.Content.Role}

		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				nm.Content += part.Text
			}
		}

		nr.Messages = append(nr.Messages, nm)

		switch candidate.FinishReason {
		case "STOP":
			nr.StopReason = "end_turn"
		case "MAX_TOKENS":
			nr.StopReason = "max_tokens"
		default:
			nr.StopReason = "end_turn"
		}
	}

	if geminiResp.UsageMetadata != nil {
		nr.Usage = core.NormalizedUsage{
			InputTokens:  geminiResp.UsageMetadata.PromptTokenCount,
			OutputTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
		}
	}

	return nr
}

// ── Helpers ───────────────────────────────────────────────────────────

func rawJSONString(s string) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return json.RawMessage(b)
}

// AnthropicToChatCompletion converts an Anthropic request to the OpenAI Chat
// Completions format used by most upstreams.
func AnthropicToChatCompletion(req *types.MessageRequest, model config.ModelConfig) (*types.ChatCompletionRequest, error) {
	return NewRequestTransformer().TransformRequest(req, model)
}
