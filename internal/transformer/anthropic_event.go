package transformer

import "fmt"

// AnthropicStreamUsage observes only token counts; other usage metadata stays
// opaque. Pointers distinguish omitted fields from explicit zero updates.
type AnthropicStreamUsage struct {
	InputTokens              *int `json:"input_tokens"`
	OutputTokens             *int `json:"output_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	ReasoningTokens          *int `json:"reasoning_tokens"`
}

// AnthropicStreamEvent shares the consumer's typed fields with source-health
// validation. Unknown content and signed extensions are not decoded or rewritten.
type AnthropicStreamEvent struct {
	Type  string                `json:"type"`
	Usage *AnthropicStreamUsage `json:"usage"`
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Message struct {
		Usage *AnthropicStreamUsage `json:"usage"`
		Model string                `json:"model"`
	} `json:"message"`
}

func (e AnthropicStreamEvent) ValidateAfterStop(stopped bool) error {
	if stopped && (e.Type == "message_start" || e.Type == "content_block_start" || e.Type == "content_block_delta" || e.Type == "content_block_stop") {
		return fmt.Errorf("Anthropic content arrived after message stop")
	}
	return nil
}
