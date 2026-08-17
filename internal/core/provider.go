// Package core defines the provider abstraction, wire format types, and
// capability metadata that form the foundation of the routing engine.
package core

import (
	"context"
	"io"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

// WireFormat describes the upstream API format a provider uses for a given model.
type WireFormat int

const (
	// WireFormatOpenAIChat is the OpenAI Chat Completions format (/v1/chat/completions).
	WireFormatOpenAIChat WireFormat = iota
	// WireFormatAnthropic is the Anthropic Messages format (/v1/messages).
	WireFormatAnthropic
	// WireFormatOpenAIResponses is the OpenAI Responses format (/v1/responses).
	WireFormatOpenAIResponses
	// WireFormatGemini is the Google Gemini format (/v1/models/{id}).
	WireFormatGemini
)

// String returns a human-readable name for the wire format.
func (w WireFormat) String() string {
	switch w {
	case WireFormatOpenAIChat:
		return "openai"
	case WireFormatAnthropic:
		return "anthropic"
	case WireFormatOpenAIResponses:
		return "responses"
	case WireFormatGemini:
		return "gemini"
	default:
		return "unknown"
	}
}

// ExecuteResult holds the result of a non-streaming provider call.
type ExecuteResult struct {
	Body []byte
}

// Provider is the abstraction for an upstream LLM provider.
type Provider interface {
	// Name returns the provider identifier (e.g. "opencode-go", "opencode-zen").
	Name() string

	// WireFormat returns the wire format for the given model on this provider.
	WireFormat(modelID string) WireFormat

	// Execute sends a non-streaming request and returns the response.
	Execute(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*ExecuteResult, error)

	// Stream sends a streaming request and returns an io.ReadCloser for SSE
	// events. The stream emits raw SSE bytes; the handler is responsible for
	// forwarding them.
	Stream(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error)
}
