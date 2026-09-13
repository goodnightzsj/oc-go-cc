// Package models provides model classification utilities shared across
// the client and provider packages. It consolidates endpoint type detection
// and model-specific classification logic to avoid duplication.
package models

import "strings"

// ModelFamily reduces an upstream model id to the bare family name the
// model-specific rules are written against. Upstreams namespace their ids
// differently - OpenCode ships "deepseek-v4-flash" and "kimi-k2.6", CommandCode
// ships "deepseek/deepseek-v4-flash" and "moonshotai/Kimi-K2.7-Code" - but the
// behaviour each family needs is the same, so normalising here keeps one owner
// for family detection instead of a second rule per provider.
//
// The result is only ever compared, never sent upstream: the wire model id stays
// the caller's original string.
func ModelFamily(modelID string) string {
	if slash := strings.LastIndex(modelID, "/"); slash >= 0 {
		modelID = modelID[slash+1:]
	}
	return strings.ToLower(modelID)
}

// EndpointType determines which API endpoint format to use for a model.
type EndpointType int

const (
	// EndpointChatCompletions is the OpenAI-compatible /v1/chat/completions endpoint.
	EndpointChatCompletions EndpointType = iota
	// EndpointAnthropic is the Anthropic /v1/messages endpoint.
	EndpointAnthropic
	// EndpointResponses is the OpenAI native /v1/responses endpoint.
	EndpointResponses
	// EndpointGemini is the Google Gemini /v1/models/{id} endpoint.
	EndpointGemini
)

// ClassifyEndpoint determines the endpoint type for a model on Zen.
// This is Zen-specific: minimax models use chat completions on Zen
// (they use Anthropic only on the Go provider).
func ClassifyEndpoint(modelID string) EndpointType {
	switch {
	case IsZenAnthropicModel(modelID):
		return EndpointAnthropic
	case IsGeminiModel(modelID):
		return EndpointGemini
	case IsResponsesModel(modelID):
		return EndpointResponses
	default:
		return EndpointChatCompletions
	}
}

// IsAnthropicModel returns true if the Go provider model requires the Anthropic endpoint.
// Most Go provider models use the Chat Completions transform path for broader
// compatibility (tool format, message roles, etc.). Exceptions are models whose
// upstream backends don't support the OpenAI Chat Completions format and only
// accept Anthropic Messages format.
//
// Only Zen models use the raw Anthropic endpoint via ClassifyEndpoint.
func IsAnthropicModel(modelID string) bool {
	switch modelID {
	case "minimax-m2.5", "minimax-m2.7", "minimax-m3",
		"qwen3.5-plus", "qwen3.6-plus", "qwen3.7-plus", "qwen3.7-max":
		return true
	default:
		return false
	}
}

// IsZenAnthropicModel returns true for models on Zen that use the Anthropic endpoint.
func IsZenAnthropicModel(modelID string) bool {
	// Claude models on Zen use the Anthropic endpoint
	if strings.HasPrefix(modelID, "claude-") {
		return true
	}
	// Qwen models on Zen use the Anthropic endpoint
	if strings.HasPrefix(modelID, "qwen") {
		return true
	}
	return false
}

// IsGeminiModel returns true for models using the Gemini endpoint.
func IsGeminiModel(modelID string) bool {
	return strings.HasPrefix(modelID, "gemini-")
}

// IsResponsesModel returns true for models using the OpenAI Responses endpoint.
func IsResponsesModel(modelID string) bool {
	return strings.HasPrefix(modelID, "gpt-") || strings.HasPrefix(modelID, "grok-") || strings.HasPrefix(modelID, "muse-spark-")
}
