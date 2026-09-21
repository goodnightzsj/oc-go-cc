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

// ClinePassProvider speaks the Cline API on behalf of the ClinePass
// subscription. It is Chat Completions only: the API publishes no Messages
// endpoint, and /v1/messages answering 401 tells us nothing, because a path
// that does not exist answers the same 401 from the same gateway.
type ClinePassProvider struct {
	baseProvider
	capture *debug.CaptureLogger
}

func NewClinePassProvider(atomic *config.AtomicConfig, capture *debug.CaptureLogger) *ClinePassProvider {
	p := &ClinePassProvider{baseProvider: newBaseProvider(atomic), capture: capture}
	// Redirects must not move a dedicated credential to another endpoint.
	p.httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return p
}

func (p *ClinePassProvider) Name() string { return "cline-pass" }

// WireFormat is constant for this platform. Unlike CommandCode, whose `claude-`
// models get native Messages, no ClinePass model has an Anthropic endpoint to
// take, so the model id cannot change the answer.
func (p *ClinePassProvider) WireFormat(string) core.WireFormat { return core.WireFormatOpenAIChat }

// Execute answers a non-streaming request.
//
// It always asks the upstream to stream and reassembles the result locally.
// This is not an optimisation: independent clients report that the Cline API
// does not reliably serve stream:false, answering an empty body or
// "generateText is not implemented", while stream:true works. What the client
// asked for is therefore not what goes upstream - but it is what comes back,
// since the reassembled document is a complete Chat Completions response, not
// SSE.
func (p *ClinePassProvider) Execute(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	body, err := p.request(ctx, req, model, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	response, err := aggregateChatStream(body, model.ModelID)
	if err != nil {
		return nil, fmt.Errorf("collect cline-pass response: %w", err)
	}
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("invalid cline-pass Chat Completions response")
	}
	converted, err := transformer.NewResponseTransformer().TransformResponse(response, model.ModelID)
	if err != nil {
		return nil, fmt.Errorf("convert cline-pass response: %w", err)
	}
	raw, err := json.Marshal(converted)
	return &core.ExecuteResult{Body: raw}, err
}

func (p *ClinePassProvider) Stream(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	return p.request(ctx, req, model, true)
}

// request sends one attempt upstream. stream is the upstream's flag; the
// caller decides it, and the value a client asked for never reaches here
// unchanged on the non-streaming path.
func (p *ClinePassProvider) request(ctx context.Context, req *types.MessageRequest, model config.ModelConfig, stream bool) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	key := p.nextAPIKey(cfg.ProviderAPIKeys(p.Name()))
	if key == "" {
		return nil, fmt.Errorf("no API key configured for cline-pass")
	}
	converted, err := transformer.AnthropicToChatCompletion(req, model)
	if err == nil {
		// The field is always written, never left to the server default: the
		// Cline API documents `stream` as defaulting to true, so relying on the
		// default would send every request down the streaming path by accident.
		converted.Stream = &stream
		var payload []byte
		payload, err = json.Marshal(converted)
		if err == nil {
			return p.send(ctx, cfg, key, payload, stream)
		}
	}
	return nil, fmt.Errorf("encode cline-pass request: %w", err)
}

func (p *ClinePassProvider) send(ctx context.Context, cfg *config.Config, key string, payload []byte, stream bool) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ClinePass.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create cline-pass request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+key)
	client.SetProviderHeaders(httpReq, p.Name())
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	if p.capture != nil {
		p.capture.CaptureUpstreamRequest(rid(ctx), p.Name(), payload)
	}
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cline-pass request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return nil, fmt.Errorf("read cline-pass error (HTTP %d): %w", resp.StatusCode, err)
		}
		return nil, &client.APIError{StatusCode: resp.StatusCode, Body: clineErrorBody(resp, body)}
	}
	if p.capture != nil {
		return client.CaptureBody(resp.Body, func(data []byte) {
			p.capture.CaptureUpstreamResponse(rid(ctx), p.Name(), data)
		}), nil
	}
	return resp.Body, nil
}

// clineErrorBody makes a gateway rejection readable when it is not JSON.
//
// The Cline edge sits behind an HTTP gateway that answers some rejections with
// a bare HTML body, so a JSON-only reader reports an empty error for exactly
// the case an operator needs to see. The status text and content type are
// substituted in that case; a body that is JSON is passed through untouched so
// nothing downstream has to re-parse a rewritten error.
func clineErrorBody(resp *http.Response, body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return string(body)
	}
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "no content type"
	}
	if len(trimmed) == 0 {
		return fmt.Sprintf("HTTP %d %s (empty %s body)", resp.StatusCode, http.StatusText(resp.StatusCode), contentType)
	}
	return fmt.Sprintf("HTTP %d %s (non-JSON %s body, %d bytes)", resp.StatusCode, http.StatusText(resp.StatusCode), contentType, len(trimmed))
}
