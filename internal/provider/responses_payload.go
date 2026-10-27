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

func responsesConfig(ctx context.Context, live *config.Config) *config.Config {
	if source := core.RequestMetadataFromContext(ctx).ResponsesSource; source != nil {
		return source
	}
	return live
}

func responsesPayload(ctx context.Context, req *types.MessageRequest, model config.ModelConfig, stream bool) (json.RawMessage, error) {
	if raw := core.RequestMetadataFromContext(ctx).ResponsesBody; len(raw) > 0 {
		return transformer.NativeResponsesPayload(raw, model.ModelID, stream)
	}
	converted, err := transformer.AnthropicToResponses(req, model)
	if err != nil {
		return nil, err
	}
	converted.Stream = stream
	return json.Marshal(converted)
}

func responsesResult(ctx context.Context, raw []byte, model config.ModelConfig, keyHint string) (*core.ExecuteResult, error) {
	var response types.ResponsesResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode Responses response: %w", err)
	}
	result := &core.ExecuteResult{KeyHint: keyHint, UpstreamModel: differingResponseModel(response.Model, model.ModelID), ResponseModel: response.Model}
	if len(core.RequestMetadataFromContext(ctx).ResponsesBody) > 0 {
		result.Body = raw
		if err := transformer.ResponsesResponseError(&response); err != nil {
			return nil, &core.NativeResponseError{ResponseError: core.ResponseError{Err: err, Usage: transformer.ResponsesUsageToAnthropic(response.Usage)}, Response: result}
		}
		return result, nil
	}
	normalized, err := transformer.ResponsesToNormalized(&response, model.ModelID)
	if err != nil {
		return nil, err
	}
	result.Body, err = json.Marshal(core.DenormalizeResponse(normalized))
	return result, err
}
