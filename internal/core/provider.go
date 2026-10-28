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
	// UpstreamChannel is the channel the upstream gateway reported serving this
	// request, when that platform publishes one. Empty for every platform that
	// does not, and empty when the gateway did not say.
	UpstreamChannel string
	// UpstreamModel preserves gateway canonical routing metadata when reported,
	// otherwise the provider's existing differing-model observation.
	UpstreamModel string
	// ResponseModel is the source response's model declaration, never a fallback
	// copied from the requested model. It may differ from the canonical model.
	ResponseModel string
	// KeyHint labels the credential that carried the request, masked to its last
	// four characters. Empty when the platform does not use a key pool.
	KeyHint string
}

// RequestConversionError marks a request rejected before an upstream call.
// Another target may support its contents, so it does not prohibit fallback.
type RequestConversionError struct {
	Err error
}

func (e *RequestConversionError) Error() string { return e.Err.Error() }
func (e *RequestConversionError) Unwrap() error { return e.Err }

// ResponsesStatusError retains a failed execution's reported usage across
// provider and fallback boundaries. Nil usage means it was not reported;
// a non-nil all-zero usage is an explicit upstream observation.
type ResponsesStatusError struct {
	Status string
	Usage  *types.Usage
}

func (e *ResponsesStatusError) Error() string { return "upstream Responses status: " + e.Status }

func (e *ResponsesStatusError) ReportedUsage() *types.Usage { return e.Usage }

// ResponseError retains a received response's usage without changing its error
// identity. Nil usage is unreported, not an observed zero.
type ResponseError struct {
	Err   error
	Usage *types.Usage
}

func (e *ResponseError) Error() string               { return e.Err.Error() }
func (e *ResponseError) Unwrap() error               { return e.Err }
func (e *ResponseError) ReportedUsage() *types.Usage { return e.Usage }

// NativeResponseError carries a complete source response that must reach the
// caller unchanged while still settling the attempt as failed.
type NativeResponseError struct {
	ResponseError
	Response *ExecuteResult
}

func (e *NativeResponseError) FinalResponse() []byte { return e.Response.Body }

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

// ModelWireFormat keeps the sender and stream decoder on the same protocol.
// Explicit model configuration takes precedence over provider inference.
//
// anthropic_tools_disabled downgrades an inferred Anthropic endpoint to Chat
// Completions. It exists because the raw Messages endpoint passes tool
// definitions through untouched, and some models only accept the Anthropic
// server-tool shorthands (`type: "custom"`) on the transformed path. The flag
// was read from config and copied through the catalog into every target, but
// nothing consulted it, so setting it changed nothing.
//
// It does not override an explicit wire_format: an operator who named the
// protocol meant it, and silently ignoring their choice is worse than a request
// that fails visibly.
func ModelWireFormat(p Provider, model config.ModelConfig) WireFormat {
	switch model.WireFormat {
	case "openai":
		return WireFormatOpenAIChat
	case "anthropic":
		return WireFormatAnthropic
	case "responses":
		return WireFormatOpenAIResponses
	case "gemini":
		return WireFormatGemini
	default:
		format := p.WireFormat(model.ModelID)
		if model.AnthropicToolsDisabled && format == WireFormatAnthropic {
			return WireFormatOpenAIChat
		}
		return format
	}
}
