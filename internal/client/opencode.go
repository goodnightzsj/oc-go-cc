// Package client manages upstream API client connections.
package client

import (
	"fmt"
	"io"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
)

// captureReadCloser wraps an io.ReadCloser with a TeeReader so every byte read
// (including streaming SSE) is also piped to the capture goroutine. Close closes
// the pipe write end, which lets the goroutine finish; the previous teeReadCloser
// never closed it, so CaptureUpstreamResponse never landed on disk and the
// goroutine leaked for every captured request.
type captureReadCloser struct {
	io.ReadCloser
	r    io.Reader
	pw   *io.PipeWriter
	done <-chan struct{}
}

func (t *captureReadCloser) Read(p []byte) (n int, err error) {
	return t.r.Read(p)
}

func (t *captureReadCloser) Close() error {
	err := t.ReadCloser.Close()
	_ = t.pw.Close()
	<-t.done
	return err
}

// CaptureBody wraps body so that all bytes read from it are delivered (async)
// to capture. Returns body unchanged when capture is nil.
func CaptureBody(body io.ReadCloser, capture func(data []byte)) io.ReadCloser {
	if capture == nil {
		return body
	}
	pr, pw := io.Pipe()
	done := make(chan struct{})
	tee := &captureReadCloser{ReadCloser: body, r: io.TeeReader(body, pw), pw: pw, done: done}
	go func() {
		defer close(done)
		defer pr.Close()
		data, _ := io.ReadAll(pr)
		capture(data)
	}()
	return tee
}

// Provider constants identify the upstream API that handles a model's request.
// These are used throughout the codebase for endpoint selection, timeout
// configuration, and provider-specific error handling (e.g., auth error
// short-circuit logic). They alias the platform registry instead of repeating
// the ids, so this package cannot end up disagreeing with config about what a
// platform is called.
const (
	ProviderOpenCodeGo  = site.OpenCodeGo
	ProviderOpenCodeZen = site.OpenCodeZen
	ProviderAWSBedrock  = site.AWSBedrock
	ProviderOpenRouter  = site.OpenRouter
	ProviderCommandCode = site.CommandCode
	ProviderClinePass   = site.ClinePass
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

// OpenCodeClient resolves per-provider request timeouts from live config.
type OpenCodeClient struct {
	atomic *config.AtomicConfig
}

// ProviderKeyCount returns the number of API keys configured for a provider.
// This is used to determine whether auth errors should short-circuit the fallback
// chain (single key) or continue trying other models (multiple keys).
func ProviderKeyCount(atomicCfg *config.AtomicConfig, provider string) int {
	if atomicCfg == nil {
		return 1 // Default to single-key behavior
	}
	return max(1, len(atomicCfg.Get().ProviderAPIKeys(provider)))
}

// NewOpenCodeClient creates a timeout resolver. Providers own HTTP transport.
func NewOpenCodeClient(atomic *config.AtomicConfig) *OpenCodeClient {
	return &OpenCodeClient{atomic: atomic}
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
	if ms := c.atomic.Get().ProviderTimeouts(Provider(modelConfig)).StreamIdleMs; ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return fallback
}

// RequestTimeout returns the provider timeout for a non-streaming attempt.
func (c *OpenCodeClient) RequestTimeout(model config.ModelConfig) time.Duration {
	if c == nil || c.atomic == nil {
		return 5 * time.Minute
	}
	if ms := c.atomic.Get().ProviderTimeouts(Provider(model)).RequestMs; ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return 5 * time.Minute
}

// StreamingTimeout returns the provider timeout for a streaming attempt.
func (c *OpenCodeClient) StreamingTimeout(model config.ModelConfig) time.Duration {
	if c == nil || c.atomic == nil {
		return 5 * time.Minute
	}
	if ms := c.atomic.Get().ProviderTimeouts(Provider(model)).StreamingTotalMs; ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return 5 * time.Minute
}

// Provider returns the provider string for a model config.
// Normalizes underscores to hyphens so that both "aws_bedrock" and "aws-bedrock"
// resolve to the same canonical form. Defaults to ProviderOpenCodeGo if empty.
func Provider(model config.ModelConfig) string {
	return config.NormalizeProvider(model.Provider)
}

// IsZen returns true if the model uses the OpenCode Zen provider.
func IsZen(model config.ModelConfig) bool {
	return Provider(model) == ProviderOpenCodeZen
}

// IsBedrock returns true if the model uses the AWS Bedrock provider.
func IsBedrock(model config.ModelConfig) bool {
	return Provider(model) == ProviderAWSBedrock
}
