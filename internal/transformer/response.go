package transformer

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

// ResponseTransformer converts OpenAI responses to Anthropic format.
type ResponseTransformer struct{}

// nonNegative clamps an integer to zero. Used when subtracting cache-token
// counts from prompt totals: defends against upstream payloads where the
// reported parts don't consistently sum to the whole.
func nonNegative(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func observeSourceModel(w interface{}, model string) {
	if recorder, ok := w.(interface{ SetSourceModel(string) }); ok && model != "" {
		recorder.SetSourceModel(model)
	}
}

// NewResponseTransformer creates a new response transformer.
func NewResponseTransformer() *ResponseTransformer {
	return &ResponseTransformer{}
}

// TransformResponse converts an OpenAI ChatCompletionResponse to Anthropic MessageResponse.
func (t *ResponseTransformer) TransformResponse(
	openaiResp *types.ChatCompletionResponse,
	originalModel string,
) (_ *types.MessageResponse, err error) {
	defer func() {
		if err != nil {
			err = &core.ResponseError{Err: err, Usage: ChatUsageToAnthropic(openaiResp.Usage)}
		}
	}()
	if err := upstreamJSONError("Chat Completions", openaiResp.Error); err != nil {
		return nil, err
	}
	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := openaiResp.Choices[0]

	// Transform content blocks from the OpenAI message.
	contentBlocks, err := t.transformContent(choice.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to transform content: %w", err)
	}

	// Map OpenAI finish reason to Anthropic stop reason.
	stopReason, err := t.mapFinishReason(choice.FinishReason)
	if err != nil {
		return nil, err
	}

	// Build Anthropic response.
	anthropicResp := &types.MessageResponse{
		ID:           openaiResp.ID,
		Type:         "message",
		Role:         "assistant",
		Content:      contentBlocks,
		Model:        originalModel,
		StopReason:   stopReason,
		StopSequence: "",
		Usage:        ChatUsageToAnthropic(openaiResp.Usage),
	}

	return anthropicResp, nil
}

func upstreamJSONError(protocol string, raw json.RawMessage) error {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	var detail struct {
		Type    string          `json:"type"`
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return fmt.Errorf("invalid %s error: %w", protocol, err)
	}
	return fmt.Errorf("upstream %s error (%s %s): %s", protocol, detail.Type, detail.Code, detail.Message)
}

// transformContent converts an OpenAI message to Anthropic content blocks.
func (t *ResponseTransformer) transformContent(msg types.ChatMessage) ([]types.ContentBlock, error) {
	var blocks []types.ContentBlock

	// Preserve reasoning content as a thinking block so it round-trips correctly
	// on multi-turn tool-calling conversations.
	if reasoning := msg.ReasoningText(); reasoning != "" {
		blocks = append(blocks, types.ContentBlock{
			Type:     "thinking",
			Thinking: reasoning,
		})
	}

	// Handle tool calls — each becomes a tool_use content block.
	for _, tc := range msg.ToolCalls {
		// Arguments come as a JSON string from OpenAI, pass as raw JSON
		inputJSON := json.RawMessage(`{}`)
		if tc.Function.Arguments != "" {
			inputJSON = json.RawMessage(tc.Function.Arguments)
		}

		blocks = append(blocks, types.ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: inputJSON,
		})
	}

	// Handle text content.
	if text := msg.ContentText(); text != "" {
		blocks = append(blocks, types.ContentBlock{
			Type: "text",
			Text: text,
		})
	}

	// Ensure at least one content block exists
	if len(blocks) == 0 {
		blocks = append(blocks, types.ContentBlock{
			Type: "text",
			Text: "",
		})
	}

	return blocks, nil
}

// mapFinishReason maps OpenAI finish reasons to Anthropic stop reasons.
func (t *ResponseTransformer) mapFinishReason(reason string) (string, error) {
	switch reason {
	case "stop":
		return "end_turn", nil
	case "length":
		return "max_tokens", nil
	case "tool_calls", "tool_use":
		return "tool_use", nil
	case "content_filter":
		return "refusal", nil
	default:
		return "", fmt.Errorf("unsupported or missing Chat Completions finish_reason %q", reason)
	}
}

// TransformErrorResponse converts an HTTP error into an Anthropic-style error map.
func TransformErrorResponse(statusCode int, message string) map[string]interface{} {
	return map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    mapHTTPStatusToErrorType(statusCode),
			"message": message,
		},
	}
}

// mapHTTPStatusToErrorType maps HTTP status codes to Anthropic error type strings.
func mapHTTPStatusToErrorType(statusCode int) string {
	switch {
	case statusCode == 400:
		return "invalid_request_error"
	case statusCode == 401:
		return "authentication_error"
	case statusCode == 403:
		return "permission_error"
	case statusCode == 404:
		return "not_found_error"
	case statusCode == 429:
		return "rate_limit_error"
	case statusCode >= 500:
		return "api_error"
	default:
		return "api_error"
	}
}
