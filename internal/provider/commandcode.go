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

// CommandCodeProvider speaks the public Provider API. Claude models use native
// Messages; all other models use Chat Completions, not Responses or private CLI APIs.
type CommandCodeProvider struct {
	baseProvider
	capture *debug.CaptureLogger
}

func NewCommandCodeProvider(atomic *config.AtomicConfig, capture *debug.CaptureLogger) *CommandCodeProvider {
	p := &CommandCodeProvider{baseProvider: newBaseProvider(atomic), capture: capture}
	// Redirects must not move a dedicated credential to another endpoint.
	p.httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return p
}

func (p *CommandCodeProvider) Name() string { return "commandcode" }

func (p *CommandCodeProvider) WireFormat(modelID string) core.WireFormat {
	if strings.HasPrefix(modelID, "claude-") {
		return core.WireFormatAnthropic
	}
	return core.WireFormatOpenAIChat
}

func (p *CommandCodeProvider) Execute(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	body, err := p.request(ctx, req, model, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read commandcode response: %w", err)
	}
	if core.ModelWireFormat(p, model) == core.WireFormatAnthropic {
		var response types.MessageResponse
		if err := json.Unmarshal(raw, &response); err != nil || response.Type != "message" {
			return nil, fmt.Errorf("invalid commandcode Messages response")
		}
		return &core.ExecuteResult{Body: raw}, nil
	}
	var response types.ChatCompletionResponse
	if err := json.Unmarshal(raw, &response); err != nil || len(response.Choices) == 0 {
		return nil, fmt.Errorf("invalid commandcode Chat Completions response")
	}
	converted, err := transformer.NewResponseTransformer().TransformResponse(&response, model.ModelID)
	if err != nil {
		return nil, fmt.Errorf("convert commandcode response: %w", err)
	}
	raw, err = json.Marshal(converted)
	return &core.ExecuteResult{Body: raw}, err
}

func (p *CommandCodeProvider) Stream(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	return p.request(ctx, req, model, true)
}

func (p *CommandCodeProvider) request(ctx context.Context, req *types.MessageRequest, model config.ModelConfig, stream bool) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	key := p.nextAPIKey(cfg.ProviderAPIKeys(p.Name()))
	if key == "" {
		return nil, fmt.Errorf("no API key configured for commandcode")
	}
	format := core.ModelWireFormat(p, model)
	endpoint := cfg.CommandCode.BaseURL
	var payload []byte
	var err error
	switch format {
	case core.WireFormatAnthropic:
		endpoint = cfg.CommandCode.AnthropicBaseURL
		payload, err = anthropicPayload(ctx, req, model, stream)
	case core.WireFormatOpenAIChat:
		var converted *types.ChatCompletionRequest
		converted, err = transformer.AnthropicToChatCompletion(req, model)
		if err == nil {
			converted.Stream = &stream
			payload, err = json.Marshal(converted)
		}
	default:
		return nil, fmt.Errorf("commandcode does not provide %s upstream format", format)
	}
	if err != nil {
		return nil, fmt.Errorf("encode commandcode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create commandcode request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+key)
	client.SetProviderHeaders(httpReq, p.Name())
	if format == core.WireFormatAnthropic {
		client.SetAnthropicHeaders(httpReq)
	}
	if cfg.CommandCode.ZeroDataRetention {
		httpReq.Header.Set("x-cmd-zdr", "1")
	}
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	if p.capture != nil {
		p.capture.CaptureUpstreamRequest(rid(ctx), p.Name(), payload)
	}
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("commandcode request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return nil, fmt.Errorf("read commandcode error (HTTP %d): %w", resp.StatusCode, err)
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
