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
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// AWSBedrockProvider implements core.Provider for the AWS Bedrock Mantle backend.
// Bedrock Mantle exposes OpenAI-compatible and optionally Anthropic-compatible endpoints.
type AWSBedrockProvider struct {
	baseProvider
}

// NewAWSBedrockProvider creates a new AWSBedrockProvider.
func NewAWSBedrockProvider(atomic *config.AtomicConfig) *AWSBedrockProvider {
	return &AWSBedrockProvider{baseProvider: newBaseProvider(atomic)}
}

// Name returns the provider identifier.
func (p *AWSBedrockProvider) Name() string { return "aws-bedrock" }

// WireFormat returns the wire format for Bedrock models.
// Bedrock Mantle uses "provider.model-name" format — models prefixed with
// "anthropic." use the Anthropic Messages endpoint, everything else uses
// OpenAI Chat Completions. The global wire_format config acts as a fallback
// only when no prefix match applies.
func (p *AWSBedrockProvider) WireFormat(modelID string) core.WireFormat {
	switch {
	case strings.HasPrefix(modelID, "anthropic."):
		return core.WireFormatAnthropic
	case strings.HasPrefix(modelID, "openai.gpt-"):
		return core.WireFormatOpenAIResponses
	case strings.HasPrefix(modelID, "openai."):
		return core.WireFormatOpenAIChat
	default:
		cfg := p.atomic.Get()
		if cfg != nil && cfg.AWSBedrock.WireFormat == "anthropic" {
			return core.WireFormatAnthropic
		}
		return core.WireFormatOpenAIChat
	}
}

func (p *AWSBedrockProvider) Execute(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	switch p.WireFormat(model.ModelID) {
	case core.WireFormatAnthropic:
		return p.executeAnthropic(ctx, req, model)
	case core.WireFormatOpenAIResponses:
		return p.executeResponses(ctx, req, model)
	default:
		return p.executeOpenAI(ctx, req, model)
	}
}

func (p *AWSBedrockProvider) Stream(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	switch p.WireFormat(model.ModelID) {
	case core.WireFormatAnthropic:
		return p.streamAnthropic(ctx, req, model)
	case core.WireFormatOpenAIResponses:
		return p.streamResponses(ctx, req, model)
	default:
		return p.streamOpenAI(ctx, req, model)
	}
}

// ── OpenAI Chat Completions ────────────────────────────────────────────

func (p *AWSBedrockProvider) executeOpenAI(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := p.bedrockEndpoint(cfg, model.ModelID)
	apiKey := p.bedrockAPIKey(cfg)

	openaiReq, err := transformer.AnthropicToChatCompletion(req, model)
	if err != nil {
		return nil, fmt.Errorf("request transform failed: %w", err)
	}
	streamFalse := false
	openaiReq.Stream = &streamFalse

	resp, err := p.doBedrockRequest(ctx, endpoint, apiKey, cfg.AWSBedrock.ProjectID, openaiReq, false)
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

func (p *AWSBedrockProvider) streamOpenAI(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := p.bedrockEndpoint(cfg, model.ModelID)
	apiKey := p.bedrockAPIKey(cfg)

	openaiReq, err := transformer.AnthropicToChatCompletion(req, model)
	if err != nil {
		return nil, fmt.Errorf("request transform failed: %w", err)
	}
	streamTrue := true
	openaiReq.Stream = &streamTrue

	resp, err := p.doBedrockRequest(ctx, endpoint, apiKey, cfg.AWSBedrock.ProjectID, openaiReq, true)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// ── OpenAI Responses API ──────────────────────────────────────────────

func (p *AWSBedrockProvider) executeResponses(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := p.responsesEndpoint(cfg)
	apiKey := p.bedrockAPIKey(cfg)

	responsesReq := p.buildResponsesRequest(req, model)

	resp, err := p.doBedrockRequest(ctx, endpoint, apiKey, cfg.AWSBedrock.ProjectID, responsesReq, false)
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

	normResp := transformer.ResponsesToNormalized(&responsesResp, model.ModelID)
	anthropicResp := core.DenormalizeResponse(normResp)
	resultBody, err := json.Marshal(anthropicResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &core.ExecuteResult{
		Body: resultBody,
	}, nil
}

func (p *AWSBedrockProvider) streamResponses(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := p.responsesEndpoint(cfg)
	apiKey := p.bedrockAPIKey(cfg)

	responsesReq := p.buildResponsesRequest(req, model)
	responsesReq.Stream = true

	resp, err := p.doBedrockRequest(ctx, endpoint, apiKey, cfg.AWSBedrock.ProjectID, responsesReq, true)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (p *AWSBedrockProvider) buildResponsesRequest(anthropicReq *types.MessageRequest, model config.ModelConfig) *types.ResponsesRequest {
	// Normalize first: Anthropic content may be a string or a block array, and
	// this endpoint expects each input's content as a plain JSON string.
	req := core.NormalizeRequest(anthropicReq)

	var inputs []types.ResponsesInput
	for _, msg := range req.Messages {
		contentBytes, _ := json.Marshal(msg.Content)
		inputs = append(inputs, types.ResponsesInput{
			Role:    msg.Role,
			Content: contentBytes,
		})
	}

	return &types.ResponsesRequest{
		Model:  model.ModelID,
		Input:  inputs,
		Stream: false,
	}
}

// ── Anthropic Messages ────────────────────────────────────────────────

func (p *AWSBedrockProvider) executeAnthropic(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.AWSBedrock.AnthropicBaseURL
	if endpoint == "" {
		return nil, fmt.Errorf("anthropic_base_url not configured for aws-bedrock provider")
	}
	apiKey := p.bedrockAPIKey(cfg)

	anthropicReq := transformer.AnthropicForModel(req, model, false)
	rawBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	if cfg.AWSBedrock.ProjectID != "" {
		httpReq.Header.Set("OpenAI-Project", cfg.AWSBedrock.ProjectID)
	}

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

func (p *AWSBedrockProvider) streamAnthropic(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.AWSBedrock.AnthropicBaseURL
	if endpoint == "" {
		return nil, fmt.Errorf("anthropic_base_url not configured for aws-bedrock provider")
	}
	apiKey := p.bedrockAPIKey(cfg)

	anthropicReq := transformer.AnthropicForModel(req, model, true)
	rawBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	if cfg.AWSBedrock.ProjectID != "" {
		httpReq.Header.Set("OpenAI-Project", cfg.AWSBedrock.ProjectID)
	}
	httpReq.Header.Set("Accept", "text/event-stream")

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

// ── Helpers ────────────────────────────────────────────────────────────

// bedrockAPIKey returns the Bedrock-specific API key if configured, otherwise
// falls back to the global API key pool.
func (p *AWSBedrockProvider) bedrockAPIKey(cfg *config.Config) string {
	if cfg.AWSBedrock.APIKey != "" {
		return cfg.AWSBedrock.APIKey
	}
	return p.nextAPIKey(cfg.EffectiveAPIKeys())
}

// needsOpenaiPath returns true for models that require the /openai path prefix
// on Bedrock Mantle. Models like xai.grok-* require this path to avoid
// "Berm is not enabled for this account" errors.
func (p *AWSBedrockProvider) needsOpenaiPath(modelID string) bool {
	return strings.HasPrefix(modelID, "xai.") || strings.HasPrefix(modelID, "openai.gpt-")
}

// bedrockEndpoint returns the appropriate endpoint for a given model.
// Some models (e.g., xai.grok) require the /openai path prefix.
func (p *AWSBedrockProvider) bedrockEndpoint(cfg *config.Config, modelID string) string {
	baseURL := cfg.AWSBedrock.BaseURL
	if p.needsOpenaiPath(modelID) && !strings.Contains(baseURL, "/openai/") {
		return strings.Replace(baseURL, "/v1/", "/openai/v1/", 1)
	}
	return baseURL
}

// responsesEndpoint returns the Responses API endpoint for models like openai.gpt-*.
func (p *AWSBedrockProvider) responsesEndpoint(cfg *config.Config) string {
	baseURL := cfg.AWSBedrock.BaseURL
	if strings.Contains(baseURL, "/chat/completions") {
		return strings.Replace(baseURL, "/chat/completions", "/responses", 1)
	}
	if strings.HasSuffix(baseURL, "/v1/") {
		return strings.TrimSuffix(baseURL, "/") + "/responses"
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/responses"
	}
	return baseURL + "/responses"
}

// doBedrockRequest sends an HTTP request to the Bedrock Mantle endpoint with
// the OpenAI-Project header when configured.
func (p *AWSBedrockProvider) doBedrockRequest(ctx context.Context, endpoint, apiKey, projectID string, req any, stream bool) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	if projectID != "" {
		httpReq.Header.Set("OpenAI-Project", projectID)
	}
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

	return resp, nil
}
