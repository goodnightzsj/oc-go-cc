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

// UpstreamChannel forwards the wrapped body's channel report. Without this the
// capture wrapper would erase the method and the channel would be lost exactly
// when capture is enabled - which is the configuration used to investigate it.
func (t *captureReadCloser) UpstreamChannel() string {
	return UpstreamChannelOf(t.ReadCloser)
}

// UpstreamModel forwards the wrapped body's model report, for the same reason
// and through the same chain as the channel above.
func (t *captureReadCloser) UpstreamModel() string {
	return UpstreamModelOf(t.ReadCloser)
}

// ServedModel forwards the first-chunk model the same way, for the same reason.
func (t *captureReadCloser) ServedModel() string {
	if reporter, ok := t.ReadCloser.(interface{ ServedModel() string }); ok {
		return reporter.ServedModel()
	}
	return ""
}

// KeyHint forwards the credential label for the same reason as the observations
// above: embedding promotes Read and Close only, so a wrapper that omits this
// erases it for everything downstream.
func (t *captureReadCloser) KeyHint() string {
	if r, ok := t.ReadCloser.(interface{ KeyHint() string }); ok {
		return r.KeyHint()
	}
	return ""
}

// MarkFirstByte and FirstByteAt forward the first-byte observation. They are
// here for the reason the methods above are: embedding promotes Read and Close
// only, so a wrapper that forwards one observation and not the others erases
// the rest. This one was missed on the first pass and the field stored empty on
// every production row - the test fixture had capture disabled, so the wrapper
// was not in the path there.
func (t *captureReadCloser) MarkFirstByte() {
	if recorder, ok := t.ReadCloser.(interface{ MarkFirstByte() }); ok {
		recorder.MarkFirstByte()
	}
}

func (t *captureReadCloser) FirstByteAt() time.Time {
	if reporter, ok := t.ReadCloser.(interface{ FirstByteAt() time.Time }); ok {
		return reporter.FirstByteAt()
	}
	return time.Time{}
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
	ProviderWorkBuddy   = site.WorkBuddy
)

// APIError represents an HTTP API error returned by an upstream provider.
// Callers should use errors.As to check for this type and inspect StatusCode
// for classification (4xx non-retryable, 5xx retryable, etc.).
type APIError struct {
	StatusCode int
	Body       string
	// RetryAfter is the upstream's Retry-After header, verbatim, when it sent
	// one. It is carried rather than parsed here because only the key scheduler
	// has a use for it, and the value may be either delta-seconds or an HTTP
	// date; the scheduler handles both.
	RetryAfter string
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

// UpstreamChannelReader is implemented by a stream body that observed which
// upstream channel actually served the request. Only platforms whose upstream
// publishes that fact implement it; everything else simply does not, and
// callers treat absence as "no channel to report".
//
// The value is only knowable once the stream has ended: the gateway names the
// channel in its last metadata frame, after the content. A caller reading it
// early gets an empty string, which is the correct answer at that point.
type UpstreamChannelReader interface {
	UpstreamChannel() string
}

// UpstreamChannelOf returns the channel a stream body reported, or "" when the
// body is not a channel reporter or has not seen the metadata yet.
//
// Callers must not reach into a concrete provider type for this: the handler
// layer would then have to import every provider package.
func UpstreamChannelOf(body any) string {
	reader, ok := body.(UpstreamChannelReader)
	if !ok {
		return ""
	}
	return reader.UpstreamChannel()
}

// UpstreamModelReader is implemented by a stream body that observed which model
// the upstream actually ran, when that differs from the id we sent.
//
// It is separate from UpstreamChannelReader because the two answer different
// questions and not every platform that reports one reports the other: a
// gateway may name its channel without rewriting the model id at all, and a
// plain upstream names neither.
type UpstreamModelReader interface {
	UpstreamModel() string
}

// UpstreamModelOf returns the model a stream body reported, or "" when the body
// does not report one or has not seen the metadata yet.
func UpstreamModelOf(body any) string {
	reader, ok := body.(UpstreamModelReader)
	if !ok {
		return ""
	}
	return reader.UpstreamModel()
}

// FirstByteAtOf returns when the upstream's first byte arrived, or the zero
// time when the body does not report one.
func FirstByteAtOf(body any) time.Time {
	reporter, ok := body.(interface{ FirstByteAt() time.Time })
	if !ok {
		return time.Time{}
	}
	return reporter.FirstByteAt()
}

// KeyHintOf returns the masked label of the credential a stream body used, or
// "" when the body does not report one.
//
// The history record needs to say which account carried a request, and this is
// the only channel that reaches it: the handler never touches the selection.
func KeyHintOf(body any) string {
	reader, ok := body.(interface{ KeyHint() string })
	if !ok {
		return ""
	}
	return reader.KeyHint()
}

// ServedModelOf returns the model the upstream named in its first chunk, which
// is available before any content is forwarded. Empty when the body does not
// report one.
func ServedModelOf(body any) string {
	reader, ok := body.(interface{ ServedModel() string })
	if !ok {
		return ""
	}
	return reader.ServedModel()
}
