package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// WorkBuddyProvider speaks to a workbuddy2api-panel instance, which presents a
// WorkBuddy account as an OpenAI-compatible API.
//
// The protocol boundary is deliberate. WorkBuddy's own API wants OAuth tokens
// that rotate hourly, a spoofed desktop identity and tool schemas rewritten to
// shapes its models accept. That gateway already does all of it and is
// maintained by someone who tracks the vendor's changes; reimplementing it here
// would mean owning that upkeep and, worse, teaching keyPool about credentials
// that expire - a concept it has no field for.
//
// Model ids are passed through untouched. The gateway names its models with a
// realm prefix (`global:deepseek-v4.1-flash`), and that prefix is part of the
// wire id exactly as `cline-pass/` is for ClinePass: stripping it would send a
// name the upstream does not recognise.
type WorkBuddyProvider struct {
	baseProvider
	capture *debug.CaptureLogger
}

func NewWorkBuddyProvider(atomic *config.AtomicConfig, capture *debug.CaptureLogger) *WorkBuddyProvider {
	base := newBaseProvider(atomic)
	registerPool(client.ProviderWorkBuddy, base.keys)
	return &WorkBuddyProvider{baseProvider: base, capture: capture}
}

func (p *WorkBuddyProvider) Name() string { return client.ProviderWorkBuddy }

// WireFormat is always Chat Completions. The gateway exposes /v1/chat/completions
// and nothing else this proxy would use, so a model configured as native
// Anthropic has to be converted rather than forwarded.
func (p *WorkBuddyProvider) WireFormat(string) core.WireFormat { return core.WireFormatOpenAIChat }

func (p *WorkBuddyProvider) Execute(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
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
	return &core.ExecuteResult{Body: raw, KeyHint: client.KeyHintOf(body)}, err
}

func (p *WorkBuddyProvider) Stream(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	return p.request(ctx, req, model, true)
}

func (p *WorkBuddyProvider) request(ctx context.Context, req *types.MessageRequest, model config.ModelConfig, stream bool) (io.ReadCloser, error) {
	if core.ModelWireFormat(p, model) != core.WireFormatOpenAIChat {
		return nil, fmt.Errorf("workbuddy supports only the openai upstream wire format")
	}
	cfg := p.atomic.Get()
	sel, err := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if err != nil {
		return nil, err
	}
	defer sel.Release()
	key := sel.Key()
	if key == "" {
		return nil, fmt.Errorf("no API key configured for provider %q", p.Name())
	}
	endpoint := chatCompletionsEndpoint(cfg.WorkBuddy.BaseURL)
	payload, err := transformer.AnthropicToChatCompletion(req, model)
	if err != nil {
		return nil, fmt.Errorf("request transform failed: %w", err)
	}
	// Always written, never left to the server default: a gateway that defaults
	// `stream` to true would turn every non-streaming call into an SSE body the
	// caller cannot parse.
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
	resp, err := p.doSelected(httpReq, sel)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() { _ = resp.Body.Close() }()
		body, readErr := io.ReadAll(resp.Body)
		retryAfter := resp.Header.Get("Retry-After")
		if readErr != nil {
			return nil, fmt.Errorf("read upstream error: %w: %w", readErr,
				&client.APIError{StatusCode: resp.StatusCode, Body: string(body)})
		}
		return nil, &client.APIError{StatusCode: resp.StatusCode, Body: string(body), RetryAfter: retryAfter}
	}
	// Observations are attached before capture, not instead of it. Wrapping in
	// the other order loses them entirely: captureReadCloser forwards neither
	// KeyHint nor FirstByteAt, so a captured body answers "" and the history
	// row records no credential and no latency while looking well-formed.
	var body io.ReadCloser = newWorkBuddyBody(resp.Body, sel.KeyHint())
	if p.capture != nil {
		body = client.CaptureBody(body, func(data []byte) {
			p.capture.CaptureUpstreamResponse(rid(ctx), p.Name(), data)
		})
	}
	return body, nil
}

// workBuddyBody stamps when the upstream's first byte arrives.
//
// Only ClinePass used to report this, because only it has a peek that reads
// ahead before handing the body over. Every other platform left first_byte_ms
// NULL, which the dashboard shows as unmeasured - indistinguishable from "this
// platform does not report it", which was true but not inevitable: the first
// byte is observable here at the same boundary, on the first read that returns
// data.
//
// The stamp is taken on the first read that yields bytes, not when the response
// headers arrive: a gateway can answer 200 and then think for several seconds,
// and that gap is exactly the figure the column exists to show.
type workBuddyBody struct {
	io.ReadCloser
	mu   sync.Mutex
	at   time.Time
	hint string
}

func newWorkBuddyBody(body io.ReadCloser, hint string) *workBuddyBody {
	return &workBuddyBody{ReadCloser: body, hint: hint}
}

func (b *workBuddyBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.mu.Lock()
		if b.at.IsZero() {
			b.at = time.Now()
		}
		b.mu.Unlock()
	}
	return n, err
}

// KeyHint reports the masked label of the credential that carried this attempt.
func (b *workBuddyBody) KeyHint() string { return b.hint }

// FirstByteAt reports when the upstream's first byte arrived, or the zero time
// when it was never observed.
func (b *workBuddyBody) FirstByteAt() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.at
}

// MarkFirstByte is part of the observation contract the handler looks for. The
// stamp is already taken on the first read, so this is a no-op kept so a caller
// that marks explicitly does not change the answer.
func (b *workBuddyBody) MarkFirstByte() {}

// chatCompletionsEndpoint resolves the configured BaseURL to the chat
// completions path.
//
// Both spellings are accepted because both are what an operator types: the
// gateway's own panel shows `http://127.0.0.1:7863`, while the OpenAI convention
// is `.../v1`. A URL that already names a path ending in /chat/completions is
// used as-is so either can be pasted without editing.
func chatCompletionsEndpoint(baseURL string) string {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if endpoint == "" {
		return ""
	}
	if strings.HasSuffix(endpoint, "/chat/completions") {
		return endpoint
	}
	if !strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/v1"
	}
	return endpoint + "/chat/completions"
}
