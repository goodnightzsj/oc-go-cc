package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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
	base := newBaseProvider(atomic)
	registerPool(client.ProviderClinePass, base.keys)
	p := &ClinePassProvider{baseProvider: base, capture: capture}
	// Redirects must not move a dedicated credential to another endpoint.
	p.httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return p
}

func (p *ClinePassProvider) Name() string { return "cline-pass" }

// Start begins the periodic channel-pin probe. It is a no-op unless pinning is
// enabled, because the probe spends a real request against a metered quota and
// there is nothing to explain when pinning is off.
//
// It is called once from main rather than from the constructor so that a
// provider built in a test never starts a background loop.
func (p *ClinePassProvider) Start(ctx context.Context) {
	go p.channelPinProbeLoop(ctx)
}

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
	// Read the channel after the body is drained: the gateway names it in the
	// last metadata frame, so it does not exist before the stream ends.
	channel := client.UpstreamChannelOf(body)
	upstreamModel := client.UpstreamModelOf(body)
	usedKey := client.KeyHintOf(body)
	// Name the model the gateway ran, not the one asked for. A gateway may
	// answer one id by running another behind its own fallback chain, and the
	// caller reading `model` from the response would otherwise be told a model
	// that never executed.
	//
	// Three sources, in order of authority. The routing block's canonicalSlug is
	// the gateway's own statement about what it ran. The aggregated stream's
	// `model` field is what the upstream chunks carried, which is available even
	// when the routing block is absent - and it was being ignored: a response
	// that named its model was overwritten with the requested id. Only when
	// neither exists does the request id stand in, which is a guess and is why
	// it goes last. ServedModel is not reachable here; the aggregate holds the
	// last chunk's value rather than the first's.
	responseModel := model.ModelID
	if response.Model != "" {
		responseModel = response.Model
	}
	if upstreamModel != "" {
		responseModel = upstreamModel
	}
	converted, err := transformer.NewResponseTransformer().TransformResponse(response, responseModel)
	if err != nil {
		return nil, fmt.Errorf("convert cline-pass response: %w", err)
	}
	raw, err := json.Marshal(converted)
	return &core.ExecuteResult{Body: raw, UpstreamChannel: channel, UpstreamModel: upstreamModel, KeyHint: usedKey}, err
}

func (p *ClinePassProvider) Stream(ctx context.Context, req *types.MessageRequest, model config.ModelConfig) (io.ReadCloser, error) {
	// The peek below reads ahead, so it needs the same idle bound the caller is
	// about to install on the body it gets back - otherwise a stalled upstream
	// is held for the platform-wide total timeout instead of the idle one.
	idleTimeout := p.streamIdleTimeout(model)
	// The returned body retains this context until Close; cancel only on failure.
	peekCtx, cancelPeek := context.WithCancel(ctx)

	body, err := p.request(peekCtx, req, model, true)
	if err != nil {
		cancelPeek()
		return nil, err
	}
	// Read ahead to the frame that names the model, then hand back a body that
	// replays it. The alternative - naming the request's model in message_start
	// and correcting it later - is impossible: message_start goes out first.
	// This costs the wait for the upstream's first byte, which the caller was
	// going to wait for anyway before writing anything.
	peeked, _, _, peekErr := peekServedModel(peekCtx, cancelPeek, body, maxServedModelPeekBytes, idleTimeout)
	if peekErr != nil {
		// Fail before message_start so the handler can still try the next model.
		cancelPeek()
		_ = body.Close()
		return nil, peekErr
	}
	// The context outlives this call because the body depends on it; closing the
	// body is what releases it, so the watchdog's cancel does not leak a
	// context that nothing would ever finish.
	return &cancelOnClose{ReadCloser: peeked, cancel: cancelPeek}, nil
}

// cancelOnClose ties the peek context's lifetime to the body's.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// The observation methods must be forwarded: embedding promotes Read and Close
// only, and every wrapper in this chain has erased one at some point.
func (c *cancelOnClose) ServedModel() string {
	if r, ok := c.ReadCloser.(interface{ ServedModel() string }); ok {
		return r.ServedModel()
	}
	return ""
}

// KeyHint forwards the credential label for the same reason as the observations
// above: embedding promotes Read and Close only, so a wrapper that omits this
// erases it for everything downstream.
func (c *cancelOnClose) KeyHint() string {
	if r, ok := c.ReadCloser.(interface{ KeyHint() string }); ok {
		return r.KeyHint()
	}
	return ""
}

func (c *cancelOnClose) UpstreamChannel() string {
	if r, ok := c.ReadCloser.(interface{ UpstreamChannel() string }); ok {
		return r.UpstreamChannel()
	}
	return ""
}

func (c *cancelOnClose) UpstreamModel() string {
	if r, ok := c.ReadCloser.(interface{ UpstreamModel() string }); ok {
		return r.UpstreamModel()
	}
	return ""
}

func (c *cancelOnClose) MarkFirstByte() {
	if r, ok := c.ReadCloser.(interface{ MarkFirstByte() }); ok {
		r.MarkFirstByte()
	}
}

func (c *cancelOnClose) FirstByteAt() time.Time {
	if r, ok := c.ReadCloser.(interface{ FirstByteAt() time.Time }); ok {
		return r.FirstByteAt()
	}
	return time.Time{}
}

// streamIdleTimeout is the gap between bytes this platform tolerates. It is the
// value the handler installs on the body it forwards, read here so the peek
// ahead of that body is held to the same bound rather than to the total.
func (p *ClinePassProvider) streamIdleTimeout(model config.ModelConfig) time.Duration {
	const fallback = 5 * time.Minute
	if p.atomic == nil {
		return fallback
	}
	if ms := p.atomic.Get().ProviderTimeouts(p.Name()).StreamIdleMs; ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return fallback
}

// request sends one attempt upstream. stream is the upstream's flag; the
// caller decides it, and the value a client asked for never reaches here
// unchanged on the non-streaming path.
func (p *ClinePassProvider) request(ctx context.Context, req *types.MessageRequest, model config.ModelConfig, stream bool) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	sel, err := p.reserveKeyChecked(p.Name(), cfg.ProviderAPIKeys(p.Name()))
	if err != nil {
		return nil, err
	}
	defer sel.Release()
	converted, err := transformer.AnthropicToChatCompletion(req, model)
	if err == nil {
		// The field is always written, never left to the server default: the
		// Cline API documents `stream` as defaulting to true, so relying on the
		// default would send every request down the streaming path by accident.
		converted.Stream = &stream
		// Cline has direct and planner pipelines. Send the restriction to both;
		// only (unlike order) excludes other channels when the gateway honors it.
		out := struct {
			*types.ChatCompletionRequest
			Provider        *clineChannelOptions `json:"provider,omitempty"`
			ProviderOptions *struct {
				Gateway clineChannelOptions `json:"gateway"`
			} `json:"providerOptions,omitempty"`
		}{ChatCompletionRequest: converted}
		if cfg.ClinePass.ChannelPinEnabled {
			target := cfg.ClinePass.ChannelPin
			direct, planner := target, target
			if target == "zai" || target == "z-ai" {
				direct, planner = "z-ai", "zai"
			}
			// The capability filter is per model because it is a hard refusal,
			// not a hint: vision is accepted for deepseek-v4.1-flash-1017 and
			// rejected for deepseek-v4-pro. A model absent from the map sends
			// no `has` at all, which is the behaviour every existing deployment
			// already has.
			has := cfg.ClinePass.ChannelPinCapabilities[model.ModelID]
			out.Provider = &clineChannelOptions{Only: []string{direct}, Has: has}
			out.ProviderOptions = &struct {
				Gateway clineChannelOptions `json:"gateway"`
			}{Gateway: clineChannelOptions{Only: []string{planner}, Has: has}}
		}
		var payload []byte
		payload, err = json.Marshal(out)
		if err == nil {
			body, sendErr := p.send(ctx, cfg, sel, payload, stream)
			// A stream body that opens successfully is a success even though the
			// caller reads it later: the upstream has accepted the key and
			// started answering, which is what the health record is about. A
			// failure that arrives mid-stream is reported by the stream reader,
			// not here.
			return body, sendErr
		}
	}
	return nil, fmt.Errorf("encode cline-pass request: %w", err)
}

func (p *ClinePassProvider) send(ctx context.Context, cfg *config.Config, sel *selection, payload []byte, stream bool) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ClinePass.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create cline-pass request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+sel.Key())
	client.SetProviderHeaders(httpReq, p.Name())
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	if p.capture != nil {
		p.capture.CaptureUpstreamRequest(rid(ctx), p.Name(), payload)
	}
	resp, err := p.doSelected(httpReq, sel)
	if err != nil {
		return nil, fmt.Errorf("cline-pass request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return nil, fmt.Errorf("read cline-pass error (HTTP %d): %w", resp.StatusCode, err)
		}
		return nil, &client.APIError{StatusCode: resp.StatusCode, Body: clineErrorBody(resp, body), RetryAfter: resp.Header.Get("Retry-After")}
	}
	// The observer is attached whether or not pinning is on. Pinning decides
	// whether we ask for a channel; it does not decide whether we may see which
	// one answered, and the caller records the observed value either way. With
	// pinning off there is no target, so the observer only reports.
	target := ""
	if cfg.ClinePass.ChannelPinEnabled {
		target = cfg.ClinePass.ChannelPin
	}
	observed := newClineChannelBody(resp.Body, target, rid(ctx))
	observed.keyHint = sel.KeyHint()
	resp.Body = observed
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
