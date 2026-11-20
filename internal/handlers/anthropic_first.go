package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/transformer"
)

const (
	availabilityBackoffStart = 30 * time.Second
	availabilityBackoffMax   = 15 * time.Minute
	maxAnthropicBodySize     = 100 << 20
	// The direct path has no per-model settings, so these mirror the proxy's
	// own defaults: a header stall fails in five minutes and a silent stream in
	// 150 seconds instead of hanging until the caller leaves.
	directResponseHeaderTimeout = 5 * time.Minute
	directIdleTimeout           = 150 * time.Second
	directTotalTimeout          = 10 * time.Minute
	maxDirectResponseBytes      = 256 << 20
)

// errClientGone marks a caller that left mid-delivery. It is not an upstream
// failure, but it is also not a success: the caller did not get a complete
// answer.
var errClientGone = errors.New("client disconnected")

// errDirectTooLarge is this proxy's own buffer limit, breached by a response it
// could not hold. It says nothing about the upstream's health, so it must not
// open the availability backoff.
var errDirectTooLarge = errors.New("direct response exceeds the local buffer limit")

// AnthropicFirstHandler sends inference to Anthropic and uses the existing
// OpenCode handler only while Anthropic is unavailable.
//
// The direct path is a passthrough: it forwards the caller's own Anthropic
// credentials and bytes, so it does not run admission, the 100/min rate
// limiter, or the platform key scheduler, and it buffers at most the same
// 100 MiB request body before forwarding. It does record what it can observe -
// one history row with usage explicitly unknown, and the in-process counters -
// so its traffic is visible in the dashboard. The config loader refuses to
// combine it with active_site, whose promise is that every request goes
// through the selected platform.
type AnthropicFirstHandler struct {
	atomic   *config.AtomicConfig
	fallback http.Handler
	client   *http.Client
	logger   *slog.Logger
	gate     availabilityGate
	storage  StorageWriter    // optional: history rows for traffic served here
	metrics  *metrics.Metrics // optional: in-process counters

	// Budgets, seeded from the constants. They are fields, not constants, only
	// so a test can drive an idle stall or an oversized body without waiting
	// 150 seconds or allocating 256 MiB.
	idleTimeout      time.Duration
	totalTimeout     time.Duration
	maxResponseBytes int64
}

type availabilityGate struct {
	mu          sync.Mutex
	baseURL     string
	unavailable bool
	probing     bool
	failures    int
	nextProbe   time.Time
	generation  uint64
}

type availabilityAttempt struct {
	baseURL    string
	probe      bool
	generation uint64
}

func (g *availabilityGate) allow(now time.Time, baseURL string) (availabilityAttempt, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.baseURL != baseURL {
		g.resetLocked(baseURL)
	}
	if !g.unavailable {
		return availabilityAttempt{baseURL: baseURL, generation: g.generation}, true
	}
	if now.Before(g.nextProbe) || g.probing {
		return availabilityAttempt{}, false
	}
	g.probing = true
	g.generation++
	return availabilityAttempt{baseURL: baseURL, probe: true, generation: g.generation}, true
}

func (g *availabilityGate) failed(now time.Time, attempt availabilityAttempt, retryAfter string) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	if attempt.baseURL != g.baseURL || attempt.generation != g.generation {
		return 0
	}

	g.unavailable = true
	if attempt.probe {
		g.probing = false
	}
	g.failures++
	delay, ok := parseRetryAfter(retryAfter, now)
	if !ok {
		delay = availabilityBackoffStart
		for i := 1; i < g.failures && delay < availabilityBackoffMax; i++ {
			delay *= 2
		}
		if delay > availabilityBackoffMax {
			delay = availabilityBackoffMax
		}
	}
	g.nextProbe = now.Add(delay)
	return delay
}

func (g *availabilityGate) available(attempt availabilityAttempt) {
	if !attempt.probe {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if attempt.baseURL != g.baseURL || attempt.generation != g.generation {
		return
	}
	g.resetLocked(g.baseURL)
}

func (g *availabilityGate) reset(baseURL string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.resetLocked(baseURL)
}

func (g *availabilityGate) resetLocked(baseURL string) {
	g.generation++
	g.baseURL = baseURL
	g.unavailable = false
	g.probing = false
	g.failures = 0
	g.nextProbe = time.Time{}
}

// abandon releases only this probe without changing the health conclusion.
func (g *availabilityGate) abandon(attempt availabilityAttempt) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if attempt.probe && attempt.generation == g.generation {
		g.probing = false
	}
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	// The header is upstream-controlled. A huge delta-seconds value overflows
	// time.Duration into the past, which would make the gate probe on every
	// request; a far-future date would park it for years. Both clamp to the
	// existing backoff ceiling.
	capDelay := func(d time.Duration) time.Duration {
		if d > availabilityBackoffMax {
			return availabilityBackoffMax
		}
		return d
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		// A zero or negative value means "retry now", which would turn the gate
		// into a probe-per-request loop. Treat it as absent and let the caller
		// apply its exponential backoff.
		if seconds <= 0 {
			return 0, false
		}
		if seconds > int(availabilityBackoffMax/time.Second) {
			return availabilityBackoffMax, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if when.Before(now) {
		// A date in the past means "retry now", the same trap as a zero
		// delta-seconds value: reopening the probe window immediately would
		// send every request to a dead upstream.
		return 0, false
	}
	return capDelay(when.Sub(now)), true
}

// NewAnthropicFirstHandler creates the opt-in Anthropic passthrough layer.
func NewAnthropicFirstHandler(atomic *config.AtomicConfig, fallback http.Handler) *AnthropicFirstHandler {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ResponseHeaderTimeout: directResponseHeaderTimeout,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
	}
	return &AnthropicFirstHandler{
		atomic:   atomic,
		fallback: fallback,
		client: &http.Client{
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		logger:           slog.Default(),
		idleTimeout:      directIdleTimeout,
		totalTimeout:     directTotalTimeout,
		maxResponseBytes: maxDirectResponseBytes,
	}
}

// SetAccounting wires history and metrics for traffic this handler serves
// itself. Optional: without them the direct path still routes, just unrecorded.
func (h *AnthropicFirstHandler) SetAccounting(storage StorageWriter, met *metrics.Metrics) {
	h.storage = storage
	h.metrics = met
}

// metricModelForDirect buckets a client-supplied model id into the fixed set
// the counters can safely hold. The raw id is unbounded and client-controlled,
// so keeping it as a map key would grow four maps without limit.
func metricModelForDirect(model string) string {
	switch {
	case strings.HasPrefix(model, "claude-opus"):
		return "claude-opus"
	case strings.HasPrefix(model, "claude-sonnet"):
		return "claude-sonnet"
	case strings.HasPrefix(model, "claude-haiku"):
		return "claude-haiku"
	case strings.HasPrefix(model, "claude"):
		return "claude"
	default:
		return "unknown"
	}
}

// recordDirect accounts for one response this handler delivered. The row
// carries no usage: the passthrough never parses the caller's body, so tokens
// and cost stay unknown (details_known=0) rather than invented. Success requires
// both a non-error status and a complete body copy.
func (h *AnthropicFirstHandler) recordDirect(requestID, model string, streaming bool, start time.Time, status int, wroteAny bool, copyErr error) {
	// A stream that delivered nothing is not a success even though it ended
	// cleanly: the caller got an empty answer and the gate must not stay warm.
	success := status < 400 && copyErr == nil && (!streaming || wroteAny)
	clientGone := errors.Is(copyErr, errClientGone)
	if h.metrics != nil {
		metricModel := metricModelForDirect(model)
		// Counted here, with the outcome, and only for traffic this path
		// actually served. Counting at entry double-counted every request that
		// fell back, because the platform handler counts the same request too.
		h.metrics.RecordRequest(streaming)
		switch {
		case success:
			h.metrics.RecordSuccess(metricModel, time.Since(start))
		case clientGone:
			// The caller left; no model was at fault. The history row still
			// records the incomplete delivery when bytes were written.
		default:
			h.metrics.RecordFailureForModel(metricModel)
		}
	}
	if h.storage == nil {
		return
	}
	if clientGone && !wroteAny && status < 400 {
		// Nothing was delivered, so there is no outcome to record - the same
		// rule the platform path applies to a pre-payload cancellation.
		return
	}
	// The model id is client-supplied; the column should not hold a 100 MiB
	// string just because the body was allowed to be that large.
	model = truncateModelID(model)
	rec := history.RequestRecord{
		// One row per execution, never the caller's correlation id: the table
		// is INSERT OR REPLACE on this key, so a reused X-Request-ID would
		// silently delete the earlier row.
		ID:             uuid.NewString(),
		RequestID:      truncateModelID(requestID),
		Model:          model,
		RequestedModel: model,
		// Not a site registry id: the registry lists the platforms this proxy
		// routes to, and several invariants require each entry to have a
		// credential source, a timeout source and a settings block. The direct
		// passthrough has none of those, so this value lives here and in the
		// dashboard's label map, and the history platform filter (which offers
		// exactly the visible platforms) cannot select it.
		Provider:        "anthropic",
		Scenario:        "anthropic-first",
		StartTime:       start,
		Duration:        time.Since(start),
		Streaming:       streaming,
		Success:         success,
		UsageUnobserved: true,
	}
	switch {
	case status >= 400:
		rec.ErrorMsg = fmt.Sprintf("anthropic returned HTTP %d", status)
	case copyErr != nil:
		rec.ErrorMsg = copyErr.Error()
	}
	if err := h.storage.InsertRequest(rec); err != nil {
		// The one writer that used to drop this error. A schema drift or a
		// read-only database would make direct traffic vanish from History
		// silently - and these rows are already the ones analytics cannot
		// reconstruct, because their usage is unknown.
		h.logger.Warn("record direct-path request", "request_id", rec.ID, "error", err)
	}
}

func (h *AnthropicFirstHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cfg := h.atomic.Get().AnthropicFirst
	if !cfg.Enabled || r.Method != http.MethodPost {
		if !cfg.Enabled {
			h.gate.reset(cfg.BaseURL)
		}
		h.fallback.ServeHTTP(w, r)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxAnthropicBodySize+1))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	if len(body) > maxAnthropicBodySize {
		h.writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}

	// The direct path does not run admission, so it owns its request id and
	// its accounting. The id also reaches the fallback chain, which reuses it.
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if len(requestID) > 256 {
		requestID = requestID[:256]
	}
	if requestID == "" {
		requestID = uuid.NewString()
	}
	w.Header().Set("X-Request-ID", requestID)
	var meta struct {
		Model  string `json:"model"`
		Stream *bool  `json:"stream"`
	}
	_ = json.Unmarshal(body, &meta)
	streaming := meta.Stream != nil && *meta.Stream
	start := time.Now()
	// An idle/stall budget for the upstream exchange plus a total cap. The
	// direct path has no per-request settings, so it uses the proxy's own
	// streaming defaults. The total cap matters because the idle timer resets
	// on every byte: a slow-drip upstream could otherwise hold the connection
	// forever.
	//
	// These budgets belong to the upstream exchange, not to the handler: r
	// keeps the caller's context. Rebinding r to idleCtx (which this did) handed
	// a dead context to serveFallback, and every platform attempt then failed
	// instantly - the caller waited out the idle budget and got a 5xx instead of
	// an answer.
	attemptCtx, cancelAttempt := context.WithTimeout(r.Context(), h.totalTimeout)
	defer cancelAttempt()
	idleCtx, cancelIdle := transformer.WithIdleCancellation(attemptCtx, cancelAttempt)
	defer cancelIdle()

	attempt, allowed := h.gate.allow(time.Now(), cfg.BaseURL)
	if !allowed {
		h.serveFallback(w, r, body, requestID)
		return
	}

	defer h.gate.abandon(attempt)
	upstreamReq, err := newAnthropicRequest(r.WithContext(idleCtx), cfg.BaseURL, body)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "invalid Anthropic base URL")
		return
	}
	resp, err := h.client.Do(upstreamReq)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		delay := h.gate.failed(time.Now(), attempt, "")
		h.logger.Warn("Anthropic unavailable, using OpenCode", "error", err, "retry_in", delay)
		h.serveFallback(w, r, body, requestID)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if isAnthropicAvailabilityFailure(resp.StatusCode) {
		delay := h.gate.failed(time.Now(), attempt, resp.Header.Get("Retry-After"))
		h.logger.Warn("Anthropic unavailable, using OpenCode", "status", resp.StatusCode, "retry_in", delay)
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		h.serveFallback(w, r, body, requestID)
		return
	}

	h.logger.Debug("Anthropic request succeeded", "status", resp.StatusCode)
	ping := transformer.StartIdleWatchdog(idleCtx, cancelIdle, h.idleTimeout)

	if !streaming {
		// Buffer before committing the status. A committed 200 with a half body
		// cannot be retried or fallen back, so a read failure must happen while
		// the response is still uncommitted.
		payload, readErr := readAllWithIdle(idleCtx, resp.Body, ping, h.maxResponseBytes)
		if readErr != nil {
			if errors.Is(readErr, errClientGone) {
				h.recordDirect(requestID, meta.Model, false, start, resp.StatusCode, false, readErr)
				return
			}
			if errors.Is(readErr, errDirectTooLarge) {
				// This proxy's buffer limit, not the upstream's health: cooling
				// the whole direct path for one response it could not hold would
				// divert every later request to the platform over nothing.
				h.logger.Warn("direct response too large to buffer, using OpenCode", "error", readErr)
				h.serveFallback(w, r, body, requestID)
				return
			}
			delay := h.gate.failed(time.Now(), attempt, "")
			h.logger.Warn("Anthropic response failed before delivery, using OpenCode", "error", readErr, "retry_in", delay)
			h.serveFallback(w, r, body, requestID)
			return
		}
		if len(payload) == 0 && resp.StatusCode >= 200 && resp.StatusCode < 300 && resp.StatusCode != http.StatusNoContent {
			// 200 with an empty body is the commonest way a broken direct path
			// looks: headers fine, nothing behind them. The status is still
			// uncommitted, so this can fall back - and it must, or every later
			// request repeats the same empty answer.
			delay := h.gate.failed(time.Now(), attempt, "")
			h.logger.Warn("Anthropic returned an empty body, using OpenCode", "retry_in", delay)
			h.serveFallback(w, r, body, requestID)
			return
		}
		// The body arrived in full: that, not the header, is what says this
		// upstream is serving. Settling before the client write also keeps a
		// caller who leaves mid-write from being read as an upstream failure.
		h.gate.available(attempt)
		copyHeader(w.Header(), resp.Header)
		writeErr := writeBufferedResponse(w, resp.StatusCode, payload)
		if writeErr != nil && r.Context().Err() != nil {
			// The write failed because the caller left. The streaming path
			// classifies this the same way; without it a disconnected client
			// was scored as a model failure.
			writeErr = errClientGone
		}
		h.recordDirect(requestID, meta.Model, false, start, resp.StatusCode, true, writeErr)
		return
	}

	copyHeader(w.Header(), resp.Header)
	if err := withStreamWriteDeadline(w, keepaliveWriteTimeout, func() error {
		w.WriteHeader(resp.StatusCode)
		return nil
	}); err != nil {
		h.logger.Debug("Anthropic response delivery failed", "error", err)
		// The caller never saw a byte; still account for the failure.
		h.recordDirect(requestID, meta.Model, streaming, start, resp.StatusCode, false, err)
		return
	}
	wroteAny, copyErr := copyStreamingResponse(idleCtx, w, resp.Body, ping)
	switch {
	case copyErr == nil && wroteAny:
		// A clean end after delivering something proves the upstream served.
		h.gate.available(attempt)
	case copyErr == nil || errors.Is(copyErr, errClientGone):
		// Nothing reached the caller. A caller that left proves nothing against
		// the upstream; an upstream that closed without a byte does.
		if !errors.Is(copyErr, errClientGone) {
			delay := h.gate.failed(time.Now(), attempt, "")
			h.logger.Warn("Anthropic stream ended without a byte", "retry_in", delay)
		}
	default:
		// The stream died under a caller who is still waiting: that is an
		// upstream failure, and cooling the path is what moves the next
		// request to the platform.
		delay := h.gate.failed(time.Now(), attempt, "")
		h.logger.Warn("Anthropic stream ended early", "error", copyErr, "retry_in", delay)
	}
	h.recordDirect(requestID, meta.Model, streaming, start, resp.StatusCode, wroteAny, copyErr)
}

// streamChunkSize matches the streaming copy's buffer, so a buffered answer and
// a streamed one reach the client in the same size pieces.
const streamChunkSize = 32 << 10

// writeBufferedResponse commits the status and then copies the payload in
// chunks, each with its own write deadline. One deadline over the whole body
// truncated a large buffered answer to a slow reader, while the streaming path
// - which renews its budget per chunk - could finish the same response.
func writeBufferedResponse(w http.ResponseWriter, status int, payload []byte) error {
	if err := withStreamWriteDeadline(w, keepaliveWriteTimeout, func() error {
		w.WriteHeader(status)
		return nil
	}); err != nil {
		return err
	}
	for len(payload) > 0 {
		chunk := payload[:min(len(payload), streamChunkSize)]
		payload = payload[len(chunk):]
		if err := withStreamWriteDeadline(w, keepaliveWriteTimeout, func() error {
			written, err := w.Write(chunk)
			if err != nil {
				return err
			}
			if written != len(chunk) {
				return io.ErrShortWrite
			}
			return http.NewResponseController(w).Flush()
		}); err != nil {
			return err
		}
	}
	return nil
}

// readAllWithIdle buffers a non-streaming body under the same idle and total
// budgets as the streaming copy.
func readAllWithIdle(ctx context.Context, body io.Reader, ping func(), limit int64) ([]byte, error) {
	var out bytes.Buffer
	buf := make([]byte, 32<<10)
	var total int64
	for {
		if ctx.Err() != nil {
			return nil, streamEndError(ctx)
		}
		n, err := body.Read(buf)
		if n > 0 {
			if ping != nil {
				ping()
			}
			total += int64(n)
			if total > limit {
				return nil, fmt.Errorf("%w of %d bytes", errDirectTooLarge, limit)
			}
			out.Write(buf[:n])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out.Bytes(), nil
			}
			if ctx.Err() != nil {
				return nil, streamEndError(ctx)
			}
			return nil, err
		}
	}
}

// truncateModelID bounds a client-supplied model id before it reaches a column
// or a log line, cutting on a rune boundary.
func truncateModelID(model string) string {
	const max = 256
	if len(model) <= max {
		return model
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(model[cut]) {
		cut--
	}
	return model[:cut]
}

func newAnthropicRequest(in *http.Request, baseURL string, body []byte) (*http.Request, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	base.Path = strings.TrimRight(base.Path, "/") + in.URL.Path
	base.RawQuery = in.URL.RawQuery
	out, err := http.NewRequestWithContext(in.Context(), in.Method, base.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	out.Header = in.Header.Clone()
	removeHopHeaders(out.Header)
	out.ContentLength = int64(len(body))
	return out, nil
}

func isAnthropicAvailabilityFailure(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func (h *AnthropicFirstHandler) serveFallback(w http.ResponseWriter, r *http.Request, body []byte, requestID string) {
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	// Hand the correlation id to the fallback chain, which reuses a request
	// header when present. The response header alone is not visible to it.
	if requestID != "" {
		r.Header.Set("X-Request-ID", requestID)
	}
	h.fallback.ServeHTTP(w, r)
}

func copyHeader(dst, src http.Header) {
	dstClone := src.Clone()
	removeHopHeaders(dstClone)
	for key, values := range dstClone {
		dst[key] = append([]string(nil), values...)
	}
}

func removeHopHeaders(header http.Header) {
	for _, value := range header.Values("Connection") {
		for _, key := range strings.Split(value, ",") {
			header.Del(strings.TrimSpace(key))
		}
	}
	for _, key := range []string{
		"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
	} {
		header.Del(key)
	}
}

var streamBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32<<10)
		return &b
	},
}

// streamEndError classifies why the copy stopped when the context is done.
// An idle watchdog cancellation is an upstream stall; anything else is the
// caller leaving.
func streamEndError(ctx context.Context) error {
	cause := context.Cause(ctx)
	if errors.Is(cause, transformer.ErrStreamIdle) {
		return transformer.ErrStreamIdle
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		// No duration here: the budget is a handler field so a test can shrink
		// it, and a constant in this message would then describe a limit that
		// was never applied.
		return fmt.Errorf("direct stream exceeded its total budget: %w", context.DeadlineExceeded)
	}
	return errClientGone
}

// copyStreamingResponse forwards the upstream body and reports how it ended:
// whether any byte reached the caller, and the terminating error. A clean EOF
// is nil; a caller cancellation is errClientGone; an upstream stall or write
// failure is returned so the accounting row does not claim success.
func copyStreamingResponse(ctx context.Context, w http.ResponseWriter, body io.Reader, ping func()) (bool, error) {
	bufPtr := streamBufPool.Get().(*[]byte)
	defer streamBufPool.Put(bufPtr)
	buf := *bufPtr
	wroteAny := false
	for {
		select {
		case <-ctx.Done():
			return wroteAny, streamEndError(ctx)
		default:
		}
		n, err := body.Read(buf)
		if n > 0 {
			if ping != nil {
				ping()
			}
			if writeErr := withStreamWriteDeadline(w, keepaliveWriteTimeout, func() error {
				written, writeErr := w.Write(buf[:n])
				if writeErr != nil {
					return writeErr
				}
				if written != n {
					return io.ErrShortWrite
				}
				return http.NewResponseController(w).Flush()
			}); writeErr != nil {
				if ctx.Err() != nil {
					return wroteAny, streamEndError(ctx)
				}
				return wroteAny, writeErr
			}
			wroteAny = true
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return wroteAny, nil
			}
			if ctx.Err() != nil {
				return wroteAny, streamEndError(ctx)
			}
			return wroteAny, err
		}
	}
}

func (h *AnthropicFirstHandler) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":  "error",
		"error": map[string]string{"type": "invalid_request_error", "message": message},
	})
}
