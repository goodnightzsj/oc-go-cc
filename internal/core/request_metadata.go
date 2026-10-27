package core

import (
	"context"
	"encoding/json"

	"github.com/routatic/proxy/internal/config"
)

// RequestMetadata carries request-scoped protocol data through every fallback.
// Bodies stay request-scoped; only explicitly selected control metadata may be logged.
type RequestMetadata struct {
	RequestID            string
	SessionID            string
	AnthropicVersion     string
	AnthropicBeta        string
	Body                 json.RawMessage
	ResponsesBody        json.RawMessage // Original Responses input; not Anthropic payload.
	ResponsesBridgeError error           // Non-nil means the routing projection cannot be sent cross-protocol.
	ResponsesReplay      bool            // Opaque reasoning must not switch models or credentials.
	ResponsesSource      *config.Config  // Immutable checked snapshot for this replay attempt; never logged.
	TargetFormat         WireFormat
}

type requestMetadataKey struct{}

func WithRequestMetadata(ctx context.Context, metadata RequestMetadata) context.Context {
	return context.WithValue(ctx, requestMetadataKey{}, metadata)
}

func RequestMetadataFromContext(ctx context.Context) RequestMetadata {
	metadata, _ := ctx.Value(requestMetadataKey{}).(RequestMetadata)
	return metadata
}
