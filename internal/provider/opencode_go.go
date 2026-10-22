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
	base := newBaseProvider(atomic)
	registerPool(client.ProviderOpenCodeGo, base.keys)
	return &OpenCodeGoProvider{baseProvider: base, capture: capture}
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
	sel, err := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if err != nil {
		return nil, err
	}
	defer sel.Release()

	openaiReq, err := transformer.AnthropicToChatCompletion(req, model)
	if err != nil {
		return nil, fmt.Errorf("request transform failed: %w", err)
	}
	streamFalse := false
	openaiReq.Stream = &streamFalse

	resp, err := p.doRequest(ctx, endpoint, sel, openaiReq, false)
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

	anthropicResp, err := transformer.NewResponseTransformer().TransformResponse(&chatResp, model.ModelID)
	if err != nil {
		return nil, fmt.Errorf("response transform failed: %w", err)
	}
	resultBody, err := json.Marshal(anthropicResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &core.ExecuteResult{
		Body:    resultBody,
		KeyHint: sel.KeyHint(),
	}, nil
}

func (p *OpenCodeGoProvider) streamOpenAI(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeGo.BaseURL
	sel, rerr := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if rerr != nil {
		return nil, rerr
	}
	defer sel.Release()

	openaiReq, err := transformer.AnthropicToChatCompletion(req, model)
	if err != nil {
		return nil, fmt.Errorf("request transform failed: %w", err)
	}
	streamTrue := true
	openaiReq.Stream = &streamTrue

	resp, err := p.doRequest(ctx, endpoint, sel, openaiReq, true)
	if err != nil {
		return nil, err
	}

	return p.withKeyHint(resp.Body, sel), nil
}

func (p *OpenCodeGoProvider) executeResponses(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	body, err := p.responsesRequest(ctx, req, model, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read Responses response: %w", err)
	}
	var response types.ResponsesResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode Responses response: %w", err)
	}
	normalized, err := transformer.ResponsesToNormalized(&response, model.ModelID)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(core.DenormalizeResponse(normalized))
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
	sel, rerr := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if rerr != nil {
		return nil, rerr
	}
	defer sel.Release()
	resp, err := p.doRequest(ctx, cfg.OpenCodeGo.ResponsesBaseURL, sel, payload, stream)
	if err != nil {
		return nil, err
	}
	return p.withKeyHint(resp.Body, sel), nil
}

// ── Anthropic Messages ────────────────────────────────────────────────

func (p *OpenCodeGoProvider) executeAnthropic(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeGo.AnthropicBaseURL
	sel, rerr := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if rerr != nil {
		return nil, rerr
	}
	defer sel.Release()

	rawBody, err := anthropicPayload(ctx, req, model, false)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+sel.Key())
	httpReq.Header.Set("x-api-key", sel.Key())
	client.SetProviderHeaders(httpReq, p.Name())
	client.SetAnthropicHeaders(httpReq)

	resp, err := p.doSelected(httpReq, sel)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		apiErr := &client.APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes), RetryAfter: resp.Header.Get("Retry-After")}
		return nil, apiErr
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
	sel, rerr := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if rerr != nil {
		return nil, rerr
	}
	defer sel.Release()

	rawBody, err := anthropicPayload(ctx, req, model, true)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+sel.Key())
	httpReq.Header.Set("x-api-key", sel.Key())
	httpReq.Header.Set("Accept", "text/event-stream")
	client.SetProviderHeaders(httpReq, p.Name())
	client.SetAnthropicHeaders(httpReq)

	if p.capture != nil {
		p.capture.CaptureUpstreamRequest(rid(ctx), p.Name(), rawBody)
	}

	resp, err := p.doSelected(httpReq, sel)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		apiErr := &client.APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes), RetryAfter: resp.Header.Get("Retry-After")}
		return nil, apiErr
	}

	if p.capture != nil {
		return client.CaptureBody(resp.Body, func(data []byte) {
			p.capture.CaptureUpstreamResponse(rid(ctx), p.Name(), data)
		}), nil
	}
	return p.withKeyHint(resp.Body, sel), nil
}

// ── HTTP helpers ──────────────────────────────────────────────────────

// rid extracts the request ID from the handler context for capture correlation.
func rid(ctx context.Context) string {
	return core.RequestMetadataFromContext(ctx).RequestID
}

// doRequest sends one request and settles the key reservation with the result,
// so every caller reports the key's health without repeating the plumbing.
func (p *OpenCodeGoProvider) doRequest(ctx context.Context, endpoint string, sel *selection, req any, stream bool) (*http.Response, error) {
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
	httpReq.Header.Set("Authorization", "Bearer "+sel.Key())
	client.SetProviderHeaders(httpReq, p.Name())
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := p.doSelected(httpReq, sel)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		apiErr := &client.APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes), RetryAfter: resp.Header.Get("Retry-After")}
		return nil, apiErr
	}

	if p.capture != nil {
		resp.Body = client.CaptureBody(resp.Body, func(data []byte) {
			p.capture.CaptureUpstreamResponse(rid(ctx), p.Name(), data)
		})
	}
	return resp, nil
}
