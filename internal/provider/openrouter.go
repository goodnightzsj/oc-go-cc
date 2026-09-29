package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// OpenRouterProvider uses Chat Completions for every model, including Claude.
type OpenRouterProvider struct {
	baseProvider
	capture *debug.CaptureLogger
}

func NewOpenRouterProvider(atomic *config.AtomicConfig, capture *debug.CaptureLogger) *OpenRouterProvider {
	return &OpenRouterProvider{baseProvider: newBaseProvider(atomic), capture: capture}
}

func (p *OpenRouterProvider) Name() string { return client.ProviderOpenRouter }

func (p *OpenRouterProvider) WireFormat(string) core.WireFormat { return core.WireFormatOpenAIChat }

func (p *OpenRouterProvider) Execute(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	body, err := p.request(ctx, req, model, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	var response types.ChatCompletionResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	converted, err := transformer.NewResponseTransformer().TransformResponse(&response, model.ModelID)
	if err != nil {
		return nil, fmt.Errorf("response transform failed: %w", err)
	}
	raw, err = json.Marshal(converted)
	return &core.ExecuteResult{Body: raw}, err
}

func (p *OpenRouterProvider) Stream(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	return p.request(ctx, req, model, true)
}

func (p *OpenRouterProvider) request(ctx context.Context, req *types.MessageRequest, model config.ModelConfig, stream bool) (io.ReadCloser, error) {
	if core.ModelWireFormat(p, model) != core.WireFormatOpenAIChat {
		return nil, fmt.Errorf("openrouter supports only the openai upstream wire format")
	}
	cfg := p.atomic.Get()
	key := p.nextAPIKey(cfg.ProviderAPIKeys(p.Name()))
	if key == "" {
		return nil, fmt.Errorf("no API key configured for provider %q", p.Name())
	}
	endpoint := strings.TrimRight(cfg.OpenRouter.BaseURL, "/")
	if strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/chat/completions"
	}
	payload, err := transformer.AnthropicToChatCompletion(req, model)
	if err != nil {
		return nil, fmt.Errorf("request transform failed: %w", err)
	}
	payload.Stream = &stream
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	if p.capture != nil {
		p.capture.CaptureUpstreamRequest(rid(ctx), p.Name(), raw)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+key)
	client.SetProviderHeaders(httpReq, p.Name())
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() { _ = resp.Body.Close() }()
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("read upstream error: %w: %w", readErr, &client.APIError{StatusCode: resp.StatusCode, Body: string(body)})
		}
		return nil, &client.APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	if p.capture != nil {
		return client.CaptureBody(resp.Body, func(data []byte) {
			p.capture.CaptureUpstreamResponse(rid(ctx), p.Name(), data)
		}), nil
	}
	return resp.Body, nil
}
