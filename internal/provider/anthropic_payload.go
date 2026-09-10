package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// Native requests retain fields not modeled by our cross-protocol DTO (for
// example tool_choice, cache controls and beta tool definitions).
func anthropicPayload(ctx context.Context, req *types.MessageRequest, model config.ModelConfig, stream bool) ([]byte, error) {
	if req.MaxTokens <= 0 {
		if model.MaxTokens <= 0 {
			return nil, fmt.Errorf("native Messages requires max_tokens in the request or model configuration")
		}
		copy := *req
		copy.MaxTokens = model.MaxTokens
		req = &copy
	}
	raw := core.RequestMetadataFromContext(ctx).Body
	if len(raw) == 0 {
		return json.Marshal(transformer.AnthropicForModel(req, model, stream))
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("invalid native Anthropic request")
	}
	fields["model"], _ = json.Marshal(model.ModelID)
	fields["stream"], _ = json.Marshal(stream)
	fields["max_tokens"], _ = json.Marshal(req.MaxTokens)
	return json.Marshal(fields)
}
