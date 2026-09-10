package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/models"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// OpenCodeGoProvider implements core.Provider for the OpenCode Go backend.
type OpenCodeGoProvider struct {
	baseProvider
	capture *debug.CaptureLogger // request/response debug capture; nil when disabled
}

// NewOpenCodeGoProvider creates a new OpenCodeGoProvider.
func NewOpenCodeGoProvider(atomic *config.AtomicConfig, capture *debug.CaptureLogger) *OpenCodeGoProvider {
	return &OpenCodeGoProvider{baseProvider: newBaseProvider(atomic), capture: capture}
}

// Name returns the provider identifier.
func (p *OpenCodeGoProvider) Name() string { return "opencode-go" }

// WireFormat returns the wire format for the given model on the Go provider.
func (p *OpenCodeGoProvider) WireFormat(modelID string) core.WireFormat {
	if models.IsAnthropicModel(modelID) {
		return core.WireFormatAnthropic
	}
	return core.WireFormatOpenAIChat
}

// Execute sends a non-streaming request and returns the response.
func (p *OpenCodeGoProvider) Execute(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	switch core.ModelWireFormat(p, model) {
	case core.WireFormatAnthropic:
		return p.executeAnthropic(ctx, req, model)
	case core.WireFormatOpenAIResponses:
		return p.executeResponses(ctx, req, model)
	default:
		return p.executeOpenAI(ctx, req, model)
	}
}

// Stream sends a streaming request and returns an io.ReadCloser for SSE events.
func (p *OpenCodeGoProvider) Stream(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	switch core.ModelWireFormat(p, model) {
	case core.WireFormatAnthropic:
		return p.streamAnthropic(ctx, req, model)
	case core.WireFormatOpenAIResponses:
		return p.streamResponses(ctx, req, model)
	default:
		return p.streamOpenAI(ctx, req, model)
	}
}

// ── OpenAI Chat Completions ────────────────────────────────────────────

func (p *OpenCodeGoProvider) executeOpenAI(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeGo.BaseURL
	apiKey := p.nextAPIKey(cfg.ProviderAPIKeys(p.Name()))

	openaiReq, err := transformer.AnthropicToChatCompletion(req, model)
	if err != nil {
		return nil, fmt.Errorf("request transform failed: %w", err)
	}
	streamFalse := false
	openaiReq.Stream = &streamFalse

	resp, err := p.doRequest(ctx, endpoint, apiKey, openaiReq, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var chatResp types.ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	normResp := transformer.OpenAIResponseToNormalized(&chatResp, model.ModelID)
	anthropicResp := core.DenormalizeResponse(normResp)
	resultBody, err := json.Marshal(anthropicResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &core.ExecuteResult{
		Body: resultBody,
	}, nil
}

func (p *OpenCodeGoProvider) streamOpenAI(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeGo.BaseURL
	apiKey := p.nextAPIKey(cfg.ProviderAPIKeys(p.Name()))

	openaiReq, err := transformer.AnthropicToChatCompletion(req, model)
	if err != nil {
		return nil, fmt.Errorf("request transform failed: %w", err)
	}
	streamTrue := true
	openaiReq.Stream = &streamTrue

	resp, err := p.doRequest(ctx, endpoint, apiKey, openaiReq, true)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (p *OpenCodeGoProvider) executeResponses(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	body, err := p.responsesRequest(ctx, req, model, false)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read Responses response: %w", err)
	}
	var response types.ResponsesResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode Responses response: %w", err)
	}
	if response.Status == "failed" || response.Status == "cancelled" {
		return nil, fmt.Errorf("upstream Responses status: %s", response.Status)
	}
	encoded, err := json.Marshal(core.DenormalizeResponse(transformer.ResponsesToNormalized(&response, model.ModelID)))
	return &core.ExecuteResult{Body: encoded}, err
}

func (p *OpenCodeGoProvider) streamResponses(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	return p.responsesRequest(ctx, req, model, true)
}

func (p *OpenCodeGoProvider) responsesRequest(ctx context.Context, req *types.MessageRequest, model config.ModelConfig, stream bool) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	payload, err := transformer.AnthropicToResponses(req, model)
	if err != nil {
		return nil, err
	}
	payload.Stream = stream
	resp, err := p.doRequest(ctx, cfg.OpenCodeGo.ResponsesBaseURL, p.nextAPIKey(cfg.ProviderAPIKeys(p.Name())), payload, stream)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// ── Anthropic Messages ────────────────────────────────────────────────

func (p *OpenCodeGoProvider) executeAnthropic(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeGo.AnthropicBaseURL
	apiKey := p.nextAPIKey(cfg.ProviderAPIKeys(p.Name()))

	rawBody, err := anthropicPayload(ctx, req, model, false)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("x-api-key", apiKey)
	client.SetProviderHeaders(httpReq, p.Name())
	client.SetAnthropicHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, &client.APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &core.ExecuteResult{Body: body}, nil
}

func (p *OpenCodeGoProvider) streamAnthropic(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeGo.AnthropicBaseURL
	apiKey := p.nextAPIKey(cfg.ProviderAPIKeys(p.Name()))

	rawBody, err := anthropicPayload(ctx, req, model, true)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	client.SetProviderHeaders(httpReq, p.Name())
	client.SetAnthropicHeaders(httpReq)

	if p.capture != nil {
		p.capture.CaptureUpstreamRequest(rid(ctx), p.Name(), rawBody)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &client.APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}

	if p.capture != nil {
		return client.CaptureBody(resp.Body, func(data []byte) {
			p.capture.CaptureUpstreamResponse(rid(ctx), p.Name(), data)
		}), nil
	}
	return resp.Body, nil
}

// ── HTTP helpers ──────────────────────────────────────────────────────

// rid extracts the request ID from the handler context for capture correlation.
func rid(ctx context.Context) string {
	return core.RequestMetadataFromContext(ctx).RequestID
}

func (p *OpenCodeGoProvider) doRequest(ctx context.Context, endpoint, apiKey string, req any, stream bool) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	if p.capture != nil {
		p.capture.CaptureUpstreamRequest(rid(ctx), p.Name(), body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	client.SetProviderHeaders(httpReq, p.Name())
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &client.APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}

	if p.capture != nil {
		resp.Body = client.CaptureBody(resp.Body, func(data []byte) {
			p.capture.CaptureUpstreamResponse(rid(ctx), p.Name(), data)
		})
	}
	return resp, nil
}
