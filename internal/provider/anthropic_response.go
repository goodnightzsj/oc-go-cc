package provider

import (
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/transformer"
)

func anthropicResult(body []byte, keyHint string) (*core.ExecuteResult, error) {
	response, err := transformer.DecodeAnthropicResponse(body)
	if err != nil {
		return nil, err
	}
	return &core.ExecuteResult{Body: body, KeyHint: keyHint, ResponseModel: response.Model}, nil
}
