// Package client manages upstream API client connections.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/models"
	"github.com/routatic/proxy/pkg/types"
)

// extractRequestID converts a context value to a string request ID.
func extractRequestID(v interface{}) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// captureReadCloser wraps an io.ReadCloser with a TeeReader so every byte read
// (including streaming SSE) is also piped to the capture goroutine. Close closes
// the pipe write end, which lets the goroutine finish; the previous teeReadCloser
// never closed it, so CaptureUpstreamResponse never landed on disk and the
// goroutine leaked for every captured request.
type captureReadCloser struct {
	io.ReadCloser
	r  io.Reader
	pw *io.PipeWriter
}

func (t *captureReadCloser) Read(p []byte) (n int, err error) {
	return t.r.Read(p)
}

func (t *captureReadCloser) Close() error {
	err := t.ReadCloser.Close()
	_ = t.pw.Close()
	return err
}

// CaptureBody wraps body so that all bytes read from it are delivered (async)
// to capture. Returns body unchanged when capture is nil.
func CaptureBody(body io.ReadCloser, capture func(data []byte)) io.ReadCloser {
	if capture == nil {
		return body
	}
	pr, pw := io.Pipe()
	tee := &captureReadCloser{ReadCloser: body, r: io.TeeReader(body, pw), pw: pw}
	go func() {
		data, _ := io.ReadAll(pr)
		capture(data)
	}()
	return tee
}

// Provider constants identify the upstream API that handles a model's request.
// These are used throughout the codebase for endpoint selection, timeout
// configuration, and provider-specific error handling (e.g., auth error
// short-circuit logic).
const (
	ProviderOpenCodeGo  = "opencode-go"
	ProviderOpenCodeZen = "opencode-zen"
	ProviderAWSBedrock  = "aws-bedrock"
	ProviderOpenRouter  = "openrouter"
)

// APIError represents an HTTP API error returned by an upstream provider.
// Callers should use errors.As to check for this type and inspect StatusCode
// for classification (4xx non-retryable, 5xx retryable, etc.).
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Body)
}

// OpenCodeClient handles communication with OpenCode Go and Zen APIs.
type OpenCodeClient struct {
	atomic        *config.AtomicConfig
	httpClient    *http.Client
	keyCounter    atomic.Uint64
	captureLogger *debug.CaptureLogger
}

// nextAPIKey returns the next API key in round-robin order from the given key pool.
// The caller provides keys from a single config read so baseURL and apiKey
// always come from the same snapshot.
func (c *OpenCodeClient) nextAPIKey(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	n := uint64(len(keys))
	old := c.keyCounter.Add(1)
	return keys[(old-1)%n]
}

// getProviderAPIKeys returns the API keys for a specific provider.
// It checks provider-specific keys first, then falls back to global keys for backward compatibility.
func (c *OpenCodeClient) getProviderAPIKeys(modelConfig config.ModelConfig) []string {
	cfg := c.atomic.Get()

	switch {
	case IsBedrock(modelConfig):
		if keys := cfg.AWSBedrock.EffectiveAPIKeys(); len(keys) > 0 {
			return keys
		}
	case IsZen(modelConfig):
		if keys := cfg.OpenCodeZen.EffectiveAPIKeys(); len(keys) > 0 {
			return keys
		}
	case IsOpenRouter(modelConfig):
		if keys := cfg.OpenRouter.EffectiveAPIKeys(); len(keys) > 0 {
			return keys
		}
	default:
		if keys := cfg.OpenCodeGo.EffectiveAPIKeys(); len(keys) > 0 {
			return keys
		}
	}

	// Fallback to global keys for backward compatibility
	return cfg.EffectiveAPIKeys()
}

// ProviderKeyCount returns the number of API keys configured for a provider.
// This is used to determine whether auth errors should short-circuit the fallback
// chain (single key) or continue trying other models (multiple keys).
func ProviderKeyCount(atomicCfg *config.AtomicConfig, provider string) int {
	if atomicCfg == nil {
		return 1 // Default to single-key behavior
	}
	cfg := atomicCfg.Get()

	var keys []string
	switch provider {
	case ProviderOpenCodeGo:
		keys = cfg.OpenCodeGo.EffectiveAPIKeys()
	case ProviderOpenCodeZen:
		keys = cfg.OpenCodeZen.EffectiveAPIKeys()
	case ProviderAWSBedrock:
		keys = cfg.AWSBedrock.EffectiveAPIKeys()
	case ProviderOpenRouter:
		keys = cfg.OpenRouter.EffectiveAPIKeys()
	default:
		// Unknown provider - default to global keys
		keys = cfg.EffectiveAPIKeys()
	}

	if len(keys) == 0 {
		return 1 // Default to single-key behavior
	}
	return len(keys)
}

// NewOpenCodeClient creates a client for sending requests to OpenCode Go,
// OpenCode Zen, or AWS Bedrock endpoints. The client handles connection
// pooling, API key rotation (round-robin across multiple keys when configured),
// and request/response capture for debugging. Pass a non-nil captureLogger
// to enable upstream traffic logging.
func NewOpenCodeClient(atomic *config.AtomicConfig, captureLogger *debug.CaptureLogger) *OpenCodeClient {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		MaxConnsPerHost:     50,
		DisableKeepAlives:   false,
		Proxy:               http.ProxyFromEnvironment,
	}

	return &OpenCodeClient{
		atomic: atomic,
		httpClient: &http.Client{
			Transport: transport,
		},
		captureLogger: captureLogger,
	}
}

// StreamIdleTimeout returns the maximum gap between bytes on an active stream
// for a model. The stream lives as long as data keeps flowing; only an idle
// period longer than this value is treated as a stuck connection and aborted.
// Go provider models use OpenCodeGo.StreamTimeoutMs; Zen models use
// OpenCodeZen.StreamTimeoutMs; Bedrock models use AWSBedrock.StreamTimeoutMs.
// Falls back to 5 minutes if the config is unavailable or the value is zero.
func (c *OpenCodeClient) StreamIdleTimeout(modelConfig config.ModelConfig) time.Duration {
	const fallback = 5 * time.Minute
	if c == nil || c.atomic == nil {
		return fallback
	}
	cfg := c.atomic.Get()
	var ms int
	switch {
	case IsBedrock(modelConfig):
		ms = cfg.AWSBedrock.StreamTimeoutMs
		if ms <= 0 {
			ms = cfg.AWSBedrock.TimeoutMs
		}
	case IsZen(modelConfig):
		ms = cfg.OpenCodeZen.StreamTimeoutMs
		if ms <= 0 {
			ms = cfg.OpenCodeZen.TimeoutMs
		}
	case IsOpenRouter(modelConfig):
		ms = cfg.OpenRouter.StreamTimeoutMs
		if ms <= 0 {
			ms = cfg.OpenRouter.TimeoutMs
		}
	default:
		ms = cfg.OpenCodeGo.StreamTimeoutMs
		if ms <= 0 {
			ms = cfg.OpenCodeGo.TimeoutMs
		}
	}
	if ms <= 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

// RequestTimeout returns the provider timeout for a non-streaming attempt.
func (c *OpenCodeClient) RequestTimeout(model config.ModelConfig) time.Duration {
	if c == nil || c.atomic == nil {
		return 5 * time.Minute
	}
	cfg := c.atomic.Get()
	var timeoutMs int
	switch {
	case IsBedrock(model):
		timeoutMs = cfg.AWSBedrock.TimeoutMs
	case IsZen(model):
		timeoutMs = cfg.OpenCodeZen.TimeoutMs
	case IsOpenRouter(model):
		timeoutMs = cfg.OpenRouter.TimeoutMs
	default:
		timeoutMs = cfg.OpenCodeGo.TimeoutMs
	}
	if timeoutMs > 0 {
		return time.Duration(timeoutMs) * time.Millisecond
	}
	return 5 * time.Minute
}

// StreamingTimeout returns the provider timeout for a streaming attempt.
func (c *OpenCodeClient) StreamingTimeout(model config.ModelConfig) time.Duration {
	if c == nil || c.atomic == nil {
		return 5 * time.Minute
	}
	cfg := c.atomic.Get()
	var timeoutMs int
	switch {
	case IsBedrock(model):
		timeoutMs = cfg.AWSBedrock.StreamingTimeoutMs
		if timeoutMs <= 0 {
			timeoutMs = cfg.AWSBedrock.TimeoutMs
		}
	case IsZen(model):
		timeoutMs = cfg.OpenCodeZen.StreamingTimeoutMs
		if timeoutMs <= 0 {
			timeoutMs = cfg.OpenCodeZen.TimeoutMs
		}
	case IsOpenRouter(model):
		timeoutMs = cfg.OpenRouter.StreamingTimeoutMs
		if timeoutMs <= 0 {
			timeoutMs = cfg.OpenRouter.TimeoutMs
		}
	default:
		timeoutMs = cfg.OpenCodeGo.StreamingTimeoutMs
		if timeoutMs <= 0 {
			timeoutMs = cfg.OpenCodeGo.TimeoutMs
		}
	}
	if timeoutMs > 0 {
		return time.Duration(timeoutMs) * time.Millisecond
	}
	return 5 * time.Minute
}

// Provider returns the provider string for a model config.
// Normalizes underscores to hyphens so that both "aws_bedrock" and "aws-bedrock"
// resolve to the same canonical form. Defaults to ProviderOpenCodeGo if empty.
func Provider(model config.ModelConfig) string {
	p := model.Provider
	if p == "" {
		return ProviderOpenCodeGo
	}
	// Normalize underscores to hyphens for consistent matching.
	for i := range p {
		if p[i] == '_' {
			// strings.ReplaceAll would allocate; do an in-place scan + build only when needed.
			return strings.ReplaceAll(p, "_", "-")
		}
	}
	return p
}

// IsZen returns true if the model uses the OpenCode Zen provider.
func IsZen(model config.ModelConfig) bool {
	return Provider(model) == ProviderOpenCodeZen
}

// IsBedrock returns true if the model uses the AWS Bedrock provider.
func IsBedrock(model config.ModelConfig) bool {
	return Provider(model) == ProviderAWSBedrock
}

// IsOpenRouter returns true if the model uses the OpenRouter provider.
func IsOpenRouter(model config.ModelConfig) bool {
	return Provider(model) == ProviderOpenRouter
}

// getEndpoint returns the appropriate endpoint config for a model.
func (c *OpenCodeClient) getEndpoint(modelID string, modelConfig config.ModelConfig) endpointConfig {
	cfg := c.atomic.Get()
	apiKey := c.nextAPIKey(c.getProviderAPIKeys(modelConfig))

	if IsBedrock(modelConfig) {
		bedrock := cfg.AWSBedrock
		return endpointConfig{BaseURL: bedrock.BaseURL, APIKey: apiKey}
	}

	if IsZen(modelConfig) {
		zen := cfg.OpenCodeZen
		switch models.ClassifyEndpoint(modelID) {
		case models.EndpointAnthropic:
			return endpointConfig{BaseURL: zen.AnthropicBaseURL, APIKey: apiKey}
		case models.EndpointResponses:
			return endpointConfig{BaseURL: zen.ResponsesBaseURL, APIKey: apiKey}
		case models.EndpointGemini:
			return endpointConfig{BaseURL: zen.GeminiBaseURL + "/" + modelID, APIKey: apiKey}
		default:
			return endpointConfig{BaseURL: zen.BaseURL, APIKey: apiKey}
		}
	}

	if IsOpenRouter(modelConfig) {
		return endpointConfig{BaseURL: cfg.OpenRouter.BaseURL, APIKey: apiKey}
	}

	// Default: OpenCode Go
	if models.IsAnthropicModel(modelID) {
		return endpointConfig{BaseURL: cfg.OpenCodeGo.AnthropicBaseURL, APIKey: apiKey}
	}
	return endpointConfig{BaseURL: cfg.OpenCodeGo.BaseURL, APIKey: apiKey}
}

// endpointConfig holds configuration for a specific API endpoint.
type endpointConfig struct {
	BaseURL string
	APIKey  string
}

// ChatCompletion sends a chat completion request.
func (c *OpenCodeClient) ChatCompletion(
	ctx context.Context,
	modelID string,
	req *types.ChatCompletionRequest,
	modelConfig config.ModelConfig,
) (*http.Response, error) {
	endpoint := c.getEndpoint(modelID, modelConfig)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Capture upstream request before sending
	if c.captureLogger != nil {
		c.captureLogger.CaptureUpstreamRequest(extractRequestID(ctx.Value("requestID")), Provider(modelConfig), body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	// Anthropic endpoint uses x-api-key; OpenAI endpoint uses Bearer
	if models.IsAnthropicModel(modelID) {
		httpReq.Header.Set("x-api-key", endpoint.APIKey)
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+endpoint.APIKey)
	}

	if req.Stream != nil && *req.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}

	// Capture upstream response by wrapping the body. CaptureBody closes the
	// pipe write end on Close so the async capture goroutine always finishes.
	if c.captureLogger != nil {
		resp.Body = CaptureBody(resp.Body, func(data []byte) {
			c.captureLogger.CaptureUpstreamResponse(extractRequestID(ctx.Value("requestID")), Provider(modelConfig), data)
		})
	}

	return resp, nil
}

// ChatCompletionNonStreaming sends a non-streaming request and returns the full parsed response.
func (c *OpenCodeClient) ChatCompletionNonStreaming(
	ctx context.Context,
	modelID string,
	req *types.ChatCompletionRequest,
	modelConfig config.ModelConfig,
) (*types.ChatCompletionResponse, error) {
	streamFalse := false
	req.Stream = &streamFalse

	resp, err := c.ChatCompletion(ctx, modelID, req, modelConfig)
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

	return &chatResp, nil
}

// GetStreamingBody returns the response body for streaming consumption.
func (c *OpenCodeClient) GetStreamingBody(
	ctx context.Context,
	modelID string,
	req *types.ChatCompletionRequest,
	modelConfig config.ModelConfig,
) (io.ReadCloser, error) {
	streamTrue := true
	req.Stream = &streamTrue

	resp, err := c.ChatCompletion(ctx, modelID, req, modelConfig)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}
