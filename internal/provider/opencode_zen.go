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
	"github.com/routatic/proxy/internal/models"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// OpenCodeZenProvider implements core.Provider for the OpenCode Zen backend.
// Zen supports four wire formats determined by model ID: Anthropic (Claude,
// Qwen), Responses (GPT), Gemini, and Chat Completions (everything else).
type OpenCodeZenProvider struct {
	baseProvider
}

// NewOpenCodeZenProvider creates a new OpenCodeZenProvider.
func NewOpenCodeZenProvider(atomic *config.AtomicConfig) *OpenCodeZenProvider {
	base := newBaseProvider(atomic)
	registerPool(client.ProviderOpenCodeZen, base.keys)
	return &OpenCodeZenProvider{baseProvider: base}
}

// Name returns the provider identifier.
func (p *OpenCodeZenProvider) Name() string { return "opencode-zen" }

// WireFormat returns the wire format for the given model on Zen.
// This replaces the old client.ClassifyEndpoint function.
func (p *OpenCodeZenProvider) WireFormat(modelID string) core.WireFormat {
	switch models.ClassifyEndpoint(modelID) {
	case models.EndpointAnthropic:
		return core.WireFormatAnthropic
	case models.EndpointGemini:
		return core.WireFormatGemini
	case models.EndpointResponses:
		return core.WireFormatOpenAIResponses
	default:
		return core.WireFormatOpenAIChat
	}
}

// Execute sends a non-streaming request and returns the response.
func (p *OpenCodeZenProvider) Execute(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	switch core.ModelWireFormat(p, model) {
	case core.WireFormatAnthropic:
		return p.executeAnthropic(ctx, req, model)
	case core.WireFormatOpenAIResponses:
		return p.executeResponses(ctx, req, model)
	case core.WireFormatGemini:
		return p.executeGemini(ctx, req, model)
	default:
		return p.executeOpenAI(ctx, req, model)
	}
}

// Stream sends a streaming request and returns an io.ReadCloser for SSE events.
func (p *OpenCodeZenProvider) Stream(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	switch core.ModelWireFormat(p, model) {
	case core.WireFormatAnthropic:
		return p.streamAnthropic(ctx, req, model)
	case core.WireFormatOpenAIResponses:
		return p.streamResponses(ctx, req, model)
	case core.WireFormatGemini:
		return p.streamGemini(ctx, req, model)
	default:
		return p.streamOpenAI(ctx, req, model)
	}
}

// ── OpenAI Chat Completions ────────────────────────────────────────────

func (p *OpenCodeZenProvider) executeOpenAI(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.BaseURL
	sel, err := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if err != nil {
		return nil, err
	}

	openaiReq, err := transformer.AnthropicToChatCompletion(req, model)
	if err != nil {
		sel.Settle(nil)
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
		Body: resultBody,
	}, nil
}

func (p *OpenCodeZenProvider) streamOpenAI(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.BaseURL
	sel, rerr := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if rerr != nil {
		return nil, rerr
	}

	openaiReq, err := transformer.AnthropicToChatCompletion(req, model)
	if err != nil {
		sel.Settle(nil)
		return nil, fmt.Errorf("request transform failed: %w", err)
	}
	streamTrue := true
	openaiReq.Stream = &streamTrue

	resp, err := p.doRequest(ctx, endpoint, sel, openaiReq, true)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// ── Anthropic Messages ────────────────────────────────────────────────

func (p *OpenCodeZenProvider) executeAnthropic(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.AnthropicBaseURL
	sel, rerr := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if rerr != nil {
		return nil, rerr
	}

	rawBody, err := anthropicPayload(ctx, req, model, false)
	if err != nil {
		sel.Settle(nil)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
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

func (p *OpenCodeZenProvider) streamAnthropic(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.AnthropicBaseURL
	sel, rerr := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if rerr != nil {
		return nil, rerr
	}

	rawBody, err := anthropicPayload(ctx, req, model, true)
	if err != nil {
		sel.Settle(nil)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
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

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &client.APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}

	return resp.Body, nil
}

// ── OpenAI Responses ──────────────────────────────────────────────────

func (p *OpenCodeZenProvider) executeResponses(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.ResponsesBaseURL
	sel, rerr := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if rerr != nil {
		return nil, rerr
	}

	responsesReq, err := transformer.AnthropicToResponses(req, model)
	if err != nil {
		sel.Settle(nil)
		return nil, err
	}
	responsesReq.Stream = false

	resp, err := p.doJSONRequest(ctx, endpoint, sel, responsesReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var responsesResp types.ResponsesResponse
	if err := json.Unmarshal(body, &responsesResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	normResp, err := transformer.ResponsesToNormalized(&responsesResp, model.ModelID)
	if err != nil {
		return nil, err
	}
	anthropicResp := core.DenormalizeResponse(normResp)
	resultBody, err := json.Marshal(anthropicResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &core.ExecuteResult{
		Body: resultBody,
	}, nil
}

func (p *OpenCodeZenProvider) streamResponses(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.ResponsesBaseURL
	sel, rerr := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if rerr != nil {
		return nil, rerr
	}

	responsesReq, err := transformer.AnthropicToResponses(req, model)
	if err != nil {
		sel.Settle(nil)
		return nil, err
	}
	responsesReq.Stream = true

	resp, err := p.doJSONRequest(ctx, endpoint, sel, responsesReq)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// ── Gemini ────────────────────────────────────────────────────────────

func (p *OpenCodeZenProvider) executeGemini(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.GeminiBaseURL + "/" + model.ModelID
	sel, rerr := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if rerr != nil {
		return nil, rerr
	}

	geminiReq := transformer.AnthropicToGemini(req, model)
	geminiReq.Stream = false

	resp, err := p.doJSONRequest(ctx, endpoint, sel, geminiReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var geminiResp types.GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	normResp := transformer.GeminiToNormalized(&geminiResp, model.ModelID)
	anthropicResp := core.DenormalizeResponse(normResp)
	resultBody, err := json.Marshal(anthropicResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &core.ExecuteResult{
		Body: resultBody,
	}, nil
}

func (p *OpenCodeZenProvider) streamGemini(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.GeminiBaseURL + "/" + model.ModelID
	sel, rerr := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if rerr != nil {
		return nil, rerr
	}

	geminiReq := transformer.AnthropicToGemini(req, model)
	geminiReq.Stream = true

	resp, err := p.doJSONRequest(ctx, endpoint, sel, geminiReq)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// ── HTTP helpers ──────────────────────────────────────────────────────

// doRequest sends one request and settles the key reservation with the result,
// so every caller reports the key's health without repeating the plumbing.
func (p *OpenCodeZenProvider) doRequest(ctx context.Context, endpoint string, sel *selection, req any, stream bool) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		sel.Settle(nil) // never sent: no verdict for the key
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+sel.Key())
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	client.SetProviderHeaders(httpReq, p.Name())

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		sel.Settle(err)
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		apiErr := &client.APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes), RetryAfter: resp.Header.Get("Retry-After")}
		sel.Settle(apiErr)
		return nil, apiErr
	}
	sel.Settle(nil)

	return resp, nil
}

func (p *OpenCodeZenProvider) doJSONRequest(ctx context.Context, endpoint string, sel *selection, req any) (*http.Response, error) {
	return p.doRequest(ctx, endpoint, sel, req, false)
}
