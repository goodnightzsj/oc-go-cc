package core

import (
	"context"
	"encoding/json"
)

// RequestMetadata carries request-scoped protocol data through every fallback.
// Body is only used for native Anthropic forwarding, never for logging headers.
type RequestMetadata struct {
	RequestID        string
	SessionID        string
	AnthropicVersion string
	AnthropicBeta    string
	Body             json.RawMessage
}

type requestMetadataKey struct{}

func WithRequestMetadata(ctx context.Context, metadata RequestMetadata) context.Context {
	return context.WithValue(ctx, requestMetadataKey{}, metadata)
}

func RequestMetadataFromContext(ctx context.Context) RequestMetadata {
	metadata, _ := ctx.Value(requestMetadataKey{}).(RequestMetadata)
	return metadata
}
