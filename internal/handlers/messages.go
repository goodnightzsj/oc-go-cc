// Package handlers contains HTTP request handlers for API endpoints.
package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/middleware"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/token"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// MessagesHandler handles /v1/messages requests.
type MessagesHandler struct {
	client           *client.OpenCodeClient // provider timeout configuration
	providerRegistry *core.ProviderRegistry
	modelRouter      *router.ModelRouter
	fallbackHandler  *router.FallbackHandler
	streamProxy      *StreamProxy // SSE proxy by wire format
	tokenCounter     *token.Counter
	logger           *slog.Logger
	rateLimiter      *middleware.RateLimiter
	requestIDGen     *middleware.RequestIDGenerator
	metrics          *metrics.Metrics
	captureLogger    *debug.CaptureLogger
	storage          StorageWriter // optional: SQLite persistence for requests/latency
}

// responseWriter wraps http.ResponseWriter to track if headers were written.
// It is safe for concurrent use: Write, WriteHeader, and Flush are serialized
// via an internal mutex so that concurrent goroutines (e.g. heartbeat and
// stream proxy) don't interleave SSE frames.
type responseWriter struct {
	http.ResponseWriter
	mu                sync.Mutex
	wroteHeader       bool
	ssePayloadWritten bool
	contentWritten    bool
	sseBuffer         []byte
	sseData           []byte
	sseEvent          string
	messageStopped    bool
	streamErr         error
	timing            streamTiming
	usage             struct {
		inputTokens              int
		outputTokens             int
		cacheReadInputTokens     int
		cacheCreationInputTokens int
		reasoningTokens          int
	}
}

func (w *responseWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.wroteHeader {
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.wroteHeader {
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(http.StatusOK)
	}
	var observed streamMilestones
	if len(b) > 0 {
		w.ssePayloadWritten = true
		var err error
		observed, err = w.observeSSE(b)
		if err != nil {
			w.streamErr = err
			return 0, err
		}
	}
	n, err := w.ResponseWriter.Write(b)
	if err == nil && n == len(b) {
		w.timing.record(observed)
	}
	return n, err
}

// observeSSE reads complete events without changing forwarded bytes. Native
// streams may split any JSON token between writes and report input/cache usage
// only in message_start; message_delta updates only fields actually present.
// Write holds mu while calling this method.
func (w *responseWriter) observeSSE(b []byte) (streamMilestones, error) {
	var observed streamMilestones
	w.sseBuffer = append(w.sseBuffer, b...)
	for {
		i := bytes.IndexByte(w.sseBuffer, '\n')
		if i < 0 {
			return observed, nil
		}
		line := strings.TrimSuffix(string(w.sseBuffer[:i]), "\r")
		w.sseBuffer = w.sseBuffer[i+1:]
		switch {
		case strings.HasPrefix(line, "event:"):
			w.sseEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			w.sseData = append(w.sseData, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")...)
			w.sseData = append(w.sseData, '\n')
		case line == "":
			if len(w.sseData) > 0 {
				event, err := w.observeSSEEvent(w.timing.firstContent.IsZero() && !observed.content)
				if err != nil {
					return streamMilestones{}, err
				}
				observed.event = observed.event || event.event
				observed.content = observed.content || event.content
				observed.end = observed.end || event.end
			}
			w.sseData = w.sseData[:0]
			w.sseEvent = ""
		}
	}
}

func (w *responseWriter) observeSSEEvent(needContent bool) (streamMilestones, error) {
	// Usage also carries non-integer metadata. Only token counts are ours to
	// parse; pointers preserve the distinction between omitted and zero deltas.
	type tokenUsage struct {
		InputTokens              *int `json:"input_tokens"`
		OutputTokens             *int `json:"output_tokens"`
		CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
		ReasoningTokens          *int `json:"reasoning_tokens"`
	}
	var event struct {
		Type    string     `json:"type"`
		Usage   tokenUsage `json:"usage"`
		Message struct {
			Usage tokenUsage `json:"usage"`
		} `json:"message"`
	}
	data := w.sseData
	w.sseData = nil
	if err := json.Unmarshal(data, &event); err != nil {
		return streamMilestones{}, fmt.Errorf("invalid upstream SSE event: %w", err)
	}
	if event.Type == "" {
		event.Type = w.sseEvent
	}
	switch event.Type {
	case "message_stop":
		w.messageStopped = true
	case "error":
		w.streamErr = errors.New("upstream reported an SSE error")
	}
	if event.Type == "content_block_start" || event.Type == "content_block_delta" {
		w.contentWritten = true
	}
	usage := event.Usage
	if event.Type == "message_start" {
		usage = event.Message.Usage
	}
	if usage.InputTokens != nil {
		w.usage.inputTokens = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		w.usage.outputTokens = *usage.OutputTokens
	}
	if usage.CacheReadInputTokens != nil {
		w.usage.cacheReadInputTokens = *usage.CacheReadInputTokens
	}
	if usage.CacheCreationInputTokens != nil {
		w.usage.cacheCreationInputTokens = *usage.CacheCreationInputTokens
	}
	if usage.ReasoningTokens != nil && *usage.ReasoningTokens > 0 {
		w.usage.reasoningTokens = *usage.ReasoningTokens
	}
	return streamMilestones{
		event:   event.Type != "" && event.Type != "ping",
		content: needContent && hasTimingContent(data, event.Type),
		end:     event.Type == "message_stop",
	}, nil
}

// finish checks the observed protocol terminal event before accounting. An
// output adapter may still fail while serializing its own terminal response.
func (w *responseWriter) finish() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.streamErr != nil {
		return w.streamErr
	}
	if !w.messageStopped || len(bytes.TrimSpace(w.sseBuffer)) > 0 || len(w.sseData) > 0 {
		return fmt.Errorf("upstream stream ended before a complete message_stop: %w", io.ErrUnexpectedEOF)
	}
	if finisher, ok := w.ResponseWriter.(interface{ Finish() error }); ok {
		return finisher.Finish()
	}
	return nil
}

// SetPartialUsage retains the last reported cumulative usage if the stream fails.
func (w *responseWriter) SetPartialUsage(usage *types.Usage) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.usage.inputTokens = usage.InputTokens
	w.usage.outputTokens = usage.OutputTokens
	w.usage.cacheReadInputTokens = usage.CacheReadInputTokens
	w.usage.cacheCreationInputTokens = usage.CacheCreationInputTokens
	if usage.ReasoningTokens > 0 {
		w.usage.reasoningTokens = usage.ReasoningTokens
	}
}

func (w *responseWriter) hasContent() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.contentWritten
}

func (w *responseWriter) getOutputTokens() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.usage.outputTokens
}

// isLowValueResponse decides whether a completed stream should be treated as a
// failure and trigger fallback. Currently this applies to long_context and
// complex scenarios when the model produced very little output.
func isLowValueResponse(scenario router.Scenario, outputTokens int, hasContent bool) bool {
	if outputTokens >= 64 {
		return false
	}
	if hasContent {
		return false
	}
	switch scenario {
	case router.ScenarioLongContext, router.ScenarioComplex:
		return true
	default:
		return false
	}
}

// headerWritten returns true if headers have been written to the response.
// Safe for concurrent use.
func (w *responseWriter) headerWritten() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.wroteHeader
}

// Flush implements http.Flusher for SSE streaming support.
// The mutex is held across the flush call to ensure Write, WriteHeader, and
// Flush remain serialized. Without this, a concurrent Flush and Write on the
// underlying http.ResponseWriter's *bufio.Writer would be a data race.
func (w *responseWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

const (
	defaultKeepaliveInterval = 3 * time.Second
	keepaliveWriteTimeout    = 5 * time.Second
)

// WriteKeepalive writes a keepalive comment frame (":keepalive\n\n") to the
// response. Unlike Write, it does NOT set ssePayloadWritten — keepalives are
// not real SSE events and should not block fallback logic on idle timeout.
func (w *responseWriter) WriteKeepalive(writeTimeout time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	controller := http.NewResponseController(w.ResponseWriter)
	if err := controller.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("set keepalive write deadline: %w", err)
	}
	if _, err := io.WriteString(w.ResponseWriter, ":keepalive\n\n"); err != nil {
		return fmt.Errorf("write keepalive: %w", err)
	}
	if err := controller.Flush(); err != nil {
		return fmt.Errorf("flush keepalive: %w", err)
	}

	// Clear the heartbeat-specific deadline after a successful flush. On error,
	// leave it in place so later response writes fail instead of blocking on the
	// same stalled client indefinitely.
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		return fmt.Errorf("clear keepalive write deadline: %w", err)
	}
	return nil
}

// startKeepaliveHeartbeat starts the streaming keepalive loop and returns a
// stop function that waits for the loop to exit. Waiting is important: canceling
// the context alone can race with a ticker event that is already flushing the
// response after the handler returns.
func startKeepaliveHeartbeat(ctx context.Context, rw *responseWriter, paused *int32, interval, writeTimeout time.Duration, logger *slog.Logger) func() {
	if interval <= 0 {
		interval = defaultKeepaliveInterval
	}
	if writeTimeout <= 0 {
		writeTimeout = keepaliveWriteTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}

	heartbeatCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if atomic.LoadInt32(paused) == 0 {
					if err := rw.WriteKeepalive(writeTimeout); err != nil {
						logger.Debug("keepalive stopped after write failure", "error", err)
						return
					}
				}
			case <-heartbeatCtx.Done():
				return
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

// NewMessagesHandler creates a new messages handler.
func NewMessagesHandler(
	openCodeClient *client.OpenCodeClient,
	providerRegistry *core.ProviderRegistry,
	modelRouter *router.ModelRouter,
	fallbackHandler *router.FallbackHandler,
	tokenCounter *token.Counter,
	metrics *metrics.Metrics,
	captureLogger *debug.CaptureLogger,
	storage StorageWriter,
) *MessagesHandler {
	return &MessagesHandler{
		client:           openCodeClient,
		providerRegistry: providerRegistry,
		modelRouter:      modelRouter,
		fallbackHandler:  fallbackHandler,
		streamProxy:      NewStreamProxy(),
		tokenCounter:     tokenCounter,
		logger:           slog.Default(),
		rateLimiter:      middleware.NewRateLimiter(100, time.Minute),
		requestIDGen:     middleware.NewRequestIDGenerator(),
		metrics:          metrics,
		captureLogger:    captureLogger,
		storage:          storage,
	}
}

type admittedRequestKey struct{}

// admitRequest is shared by protocol entry points. Only the internal adapter
// can carry this marker, so a Responses request consumes the same bucket once.
func (h *MessagesHandler) admitRequest(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if requestID, ok := r.Context().Value(admittedRequestKey{}).(string); ok {
		w.Header().Set("X-Request-ID", requestID)
		return r, true
	}

	// Generate or get request ID for correlation.
	// Cap externally-provided IDs at 256 bytes to prevent header abuse.
	requestID := r.Header.Get("X-Request-ID")
	if len(requestID) > 256 {
		requestID = requestID[:256]
	}
	if requestID == "" {
		requestID = h.requestIDGen.Generate()
	}
	w.Header().Set("X-Request-ID", requestID)

	// Rate limiting
	clientIP := middleware.GetClientIP(r)
	if !h.rateLimiter.Allow(clientIP) {
		h.metrics.RecordRateLimited()
		h.logger.Warn("rate limited", "client", clientIP, "request_id", requestID)
		return r, false
	}
	return r.WithContext(context.WithValue(r.Context(), admittedRequestKey{}, requestID)), true
}

// HandleMessages handles POST /v1/messages.
func (h *MessagesHandler) HandleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	admitted, allowed := h.admitRequest(w, r)
	if !allowed {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	r = admitted
	requestID := w.Header().Get("X-Request-ID")

	// Read the raw request body with a size limit to prevent memory exhaustion.
	const maxBodySize = 104857600 // 100 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var rawBody json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.sendError(w, http.StatusRequestEntityTooLarge, "request body too large", err)
			return
		}
		h.sendError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if h.captureLogger != nil {
		h.captureLogger.CaptureOriginal(requestID, rawBody)
	}

	// Parse into Anthropic request
	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// Validate request
	if err := anthropicReq.Validate(); err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	sessionID := r.Header.Get("x-claude-code-session-id")
	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	r = r.WithContext(core.WithRequestMetadata(r.Context(), core.RequestMetadata{
		RequestID: requestID, SessionID: sessionID, Body: rawBody,
		AnthropicVersion: r.Header.Get("anthropic-version"),
		AnthropicBeta:    strings.Join(r.Header.Values("anthropic-beta"), ","),
	}))

	// Record metrics
	isStreaming := anthropicReq.Stream != nil && *anthropicReq.Stream
	h.metrics.RecordRequest(isStreaming)

	h.logger.Info("received request",
		"model", anthropicReq.Model,
		"streaming", isStreaming,
		"messages", len(anthropicReq.Messages),
		"tools", len(anthropicReq.Tools),
		"max_tokens", anthropicReq.MaxTokens,
	)

	// Build message content for routing and token counting.
	var routerMessages []router.MessageContent
	var tokenMessages []token.MessageContent
	systemText := anthropicReq.SystemText()
	needsTools := len(anthropicReq.Tools) > 0

	for _, msg := range anthropicReq.Messages {
		blocks := msg.ContentBlocks()
		content := extractTextFromBlocks(blocks)
		mc := router.MessageContent{
			Role:          msg.Role,
			Content:       content,
			HasImage:      blocksHaveImage(blocks),
			ToolsDeclared: needsTools,
			ImageHashes:   imageHashesFromBlocks(blocks),
		}
		routerMessages = append(routerMessages, mc)
		tokenMessages = append(tokenMessages, token.MessageContent{
			Role:        msg.Role,
			Content:     content,
			ExtraTokens: imageTokenEstimate(blocks),
		})
	}

	tokenCount := h.tokenCounter.CountMessages(systemText, tokenMessages)

	// Route to appropriate model and build fallback chain.
	facts := router.AnalyzeRequestFacts(routerMessages)
	modelChain, routeResult, err := h.buildModelChain(r.Context(), anthropicReq.Model, routerMessages, tokenCount, isStreaming, anthropicReq.MaxTokens, facts.NeedsVision, needsTools)
	if err != nil {
		status := http.StatusInternalServerError
		message := "routing failed"
		if errors.Is(err, router.ErrUnknownProvider) {
			status = http.StatusBadRequest
			message = err.Error()
		}
		h.sendError(w, status, message, err)
		return
	}

	h.logger.Info("routing request",
		"scenario", routeResult.Scenario,
		"model", routeResult.Primary.ModelID,
		"provider", routeResult.Primary.Provider,
		"tokens", tokenCount,
	)

	// The normalized form exists only for the debug capture; providers consume
	// the Anthropic request directly.
	if h.captureLogger != nil && len(modelChain) > 0 {
		normalized := core.NormalizeRequest(&anthropicReq)
		normalized.Stream = isStreaming
		data, _ := json.Marshal(normalized)
		h.captureLogger.CaptureNormalized(requestID, modelChain[0].Provider, data)
	}

	if isStreaming {
		h.handleStreaming(w, r, &anthropicReq, modelChain, rawBody, routeResult.Scenario, requestID)
	} else {
		h.handleNonStreaming(w, r, &anthropicReq, modelChain, rawBody, routeResult.Scenario, requestID)
	}
}

// buildModelChain resolves the request to a model chain (primary + fallbacks),
// honoring model_overrides (with a deduplicated scenario safety-net) and
// respecting the streaming-scenario-routing toggle.
//
// Precedence:
//  1. If requestedModel matches an entry in model_overrides (exact) or
//     model_family_overrides (family keyword substring, e.g. "opus"), use that
//     as the primary and append the scenario chain as a deduplicated safety net.
//     Exact overrides win over family matches.
//  2. Otherwise, fall through to scenario-based routing via routeOnce.
//
// keepUsableFallbacks drops a fallback whose platform holds no credential.
//
// A cross-platform fallback is only a plan if this instance can actually reach
// that platform. Keeping a keyless entry would produce a chain that reads as
// protected and fails on the attempt that needed it, which is the same silent
// gap the fallback feature is supposed to close.
func (h *MessagesHandler) keepUsableFallbacks(fallbacks []config.ModelConfig) []config.ModelConfig {
	if len(fallbacks) == 0 {
		return fallbacks
	}
	cfg := h.modelRouter.Config()
	out := make([]config.ModelConfig, 0, len(fallbacks))
	for _, target := range fallbacks {
		if len(cfg.ProviderAPIKeys(client.Provider(target))) == 0 {
			h.logger.Info("fallback skipped: platform has no credential",
				"model", target.ModelID, "provider", client.Provider(target))
			continue
		}
		out = append(out, target)
	}
	return out
}

func (h *MessagesHandler) buildModelChain(
	ctx context.Context,
	requestedModel string,
	routerMessages []router.MessageContent,
	tokenCount int,
	isStreaming bool,
	requestedMaxTokens int,
	needsVision bool,
	needsTools bool,
) ([]config.ModelConfig, router.RouteResult, error) {
	var chain []config.ModelConfig
	var result router.RouteResult

	// The active site is the authority for its own model names, so a client that
	// picked a model from the listing gets exactly that model on that platform.
	// This runs before the override lookup on purpose: the listing is what the
	// client chose from, and re-mapping its choice through a configured alias
	// would send the request somewhere the client did not ask for.
	if primary, ok := h.modelRouter.PublishedByActiveSite(ctx, requestedModel); ok {
		// The scenario chain stays as a safety net, but only within the active
		// site: falling back to another platform is what the scope exists to
		// prevent.
		fallbacks, _ := h.routeOnce(routerMessages, tokenCount, "", isStreaming)
		configured := fallbacks.GetModelChain()
		net := router.RestrictToActiveSite(h.modelRouter.ActiveSite(), configured)
		if len(configured) > 0 && len(net) == 0 {
			// Configured but unreachable. Without this the operator sees a chain
			// in the dashboard and no fallback in practice, with nothing saying
			// why: the active site scopes routing, and every configured fallback
			// belonged to another platform.
			h.logger.Warn("configured fallbacks are outside the active site and cannot be used",
				"active_site", h.modelRouter.ActiveSite(), "configured", len(configured))
		}
		result = router.RouteResult{Primary: primary, Scenario: router.ScenarioOverride}
		chain = appendUniqueModels([]config.ModelConfig{primary}, net)
	} else if requestedModel != "" {
		overrideResult, ok := h.modelRouter.RouteWithOverride(requestedModel)
		if !ok {
			overrideResult, ok = h.modelRouter.RouteWithFamilyOverride(requestedModel)
		}
		if ok {
			scenarioResult, err := h.routeOnce(routerMessages, tokenCount, "", isStreaming)
			if err != nil {
				return overrideResult.GetModelChain(), overrideResult, err
			}
			chain = appendUniqueModels(overrideResult.GetModelChain(), scenarioResult.GetModelChain())
			result = overrideResult
		}
	}

	if chain == nil {
		var err error
		result, err = h.routeOnce(routerMessages, tokenCount, requestedModel, isStreaming)
		if err != nil {
			return nil, result, err
		}
		chain = result.GetModelChain()
	}

	decision, err := router.FilterByCapacity(chain, tokenCount, requestedMaxTokens, needsVision, needsTools)
	if err != nil {
		return nil, result, err
	}

	for _, s := range decision.Skipped {
		h.logger.Info("model skipped by capacity filter", "model", s.ModelID, "reason", s.Reason)
	}

	// The primary is scoped to the active site: sending the request to another
	// platform by default would not be a fallback, it would be a different
	// decision than the one the operator made. Fallbacks are not scoped - a
	// plan for "this platform cannot answer" that is limited to this platform
	// cannot do its job. Same-platform entries stay ahead of cross-platform
	// ones, so quota already paid for is spent first.
	if active := h.modelRouter.ActiveSite(); active != "" {
		if len(decision.Models) == 0 {
			return nil, result, fmt.Errorf("%w: active site %q has no routing target for this request; configure a model for it or switch the site", router.ErrWithinSiteNoTarget, active)
		}
		primary := decision.Models[0]
		if client.Provider(primary) != active {
			return nil, result, fmt.Errorf("%w: active site %q has no routing target for this request; the resolved primary belongs to %q", router.ErrWithinSiteNoTarget, active, client.Provider(primary))
		}
		fallbacks := h.keepUsableFallbacks(decision.Models[1:])
		crossCount, ordered := router.ClassifyChain(active, primary, fallbacks)
		if crossCount > 0 {
			// Recorded either way. With the switch off this is the only way to
			// learn whether the feature would ever fire - the count is measured,
			// nothing is spent, and the decision is made on data.
			enabled := h.modelRouter.Config().EnableCrossPlatformFallback
			h.logger.Info("cross-platform fallback candidates",
				"active_site", active, "candidates", crossCount, "enabled", enabled)
			if !enabled {
				// Keep only what stays inside the active site. The chain is
				// truncated rather than flagged so no execution path has to know
				// about the switch.
				ordered = router.RestrictToActiveSite(active, ordered)
			}
		}
		decision.Models = append([]config.ModelConfig{primary}, ordered...)
	}

	return decision.Models, result, nil
}

// routeOnce performs scenario-based routing, honoring the streaming-scenario-routing
// toggle. Pass requestedModel="" to force scenario routing (used for the override
// safety-net chain), or a non-empty value to let resolveRequestedModel kick in
// (only when respect_requested_model is enabled and no override matched).
func (h *MessagesHandler) routeOnce(
	routerMessages []router.MessageContent,
	tokenCount int,
	requestedModel string,
	isStreaming bool,
) (router.RouteResult, error) {
	if isStreaming && !h.modelRouter.IsStreamingScenarioRoutingEnabled() {
		// Streaming: use faster models to minimize TTFT (time-to-first-token)
		return h.modelRouter.RouteForStreaming(routerMessages, tokenCount, requestedModel)
	}
	return h.modelRouter.Route(routerMessages, tokenCount, requestedModel)
}

// appendUniqueModels keeps the first target per provider/model pair. Models
// with the same ID on different providers remain independent fallbacks.
func appendUniqueModels(base, extra []config.ModelConfig) []config.ModelConfig {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base))
	for _, m := range base {
		seen[config.ModelKey(m)] = struct{}{}
	}
	for _, m := range extra {
		if _, ok := seen[config.ModelKey(m)]; ok {
			continue
		}
		base = append(base, m)
		seen[config.ModelKey(m)] = struct{}{}
	}
	return base
}

// handleStreaming handles a streaming request with real-time SSE proxying.
func (h *MessagesHandler) handleStreaming(
	w http.ResponseWriter,
	r *http.Request,
	anthropicReq *types.MessageRequest,
	modelChain []config.ModelConfig,
	rawBody json.RawMessage,
	scenario router.Scenario,
	requestID string,
) {
	clientCtx := h.metrics.WithUpstreamCounting(r.Context())
	// The model the client named, recorded alongside the one that served it so a
	// rerouted request can be told apart from a passthrough after the fact.
	requestedModel := anthropicReq.Model
	// A caller's correlation ID may be reused; each billed execution is distinct.
	recordID := uuid.NewString()

	rw := &responseWriter{ResponseWriter: w}

	// Set SSE headers immediately
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rw.WriteHeader(http.StatusOK)
	rw.Flush()

	// Start heartbeat and wait for it to stop before the handler returns so the
	// HTTP server cannot finalize the response writer during a keepalive flush.
	var heartbeatPaused int32
	stopHeartbeat := startKeepaliveHeartbeat(clientCtx, rw, &heartbeatPaused, defaultKeepaliveInterval, keepaliveWriteTimeout, h.logger)
	defer stopHeartbeat()

	streamStart := time.Now()
	// What the client asked for, before any provider decides whether it can
	// honour it. Read once here so both record paths use one value.
	clientEffort := history.RequestedEffort(anthropicReq)
	blockedProviders := make(map[string]bool)
	// lastStreamErr keeps the last refusal so the terminal response can name the
	// cause. Without it every exhausted chain reads as "all streaming models
	// failed", which says nothing a client can act on.
	var lastStreamErr error
	// currentStream is the attempt's upstream body, held here so the failure
	// recorder can read the channel from it. The gateway names the channel in
	// its last frame, so this is only ever read after the stream has stopped -
	// which is exactly when a failure is recorded.
	var currentStream io.ReadCloser

	for attempt, model := range modelChain {
		select {
		case <-clientCtx.Done():
			h.logger.Debug("client disconnected, stopping streaming fallbacks")
			return
		default:
		}
		providerName := client.Provider(model)
		if blockedProviders[providerName] {
			h.logger.Info("provider usage limit reached, skipping streaming model", "provider", providerName, "model", model.ModelID)
			continue
		}
		complete := h.fallbackHandler.AllowAttempt(model)
		if complete == nil {
			h.logger.Info("circuit breaker open, skipping streaming model", "provider", providerName, "model", model.ModelID)
			continue
		}

		h.logger.Info("attempting streaming model", "model", model.ModelID, "provider", model.Provider)

		// Upstream context carries the streaming timeout configured for the model.
		timeout := h.client.StreamingTimeout(model)
		attemptCtx, cancelAttempt := context.WithTimeout(clientCtx, timeout)
		idleTimeout := h.client.StreamIdleTimeout(model)
		var generation *metrics.GenerationObservation

		// recordStreamSuccess records a successful stream completion and
		// marks the model attempt as done.
		//
		// The channel is read from the body rather than from a captured
		// variable: the gateway names it in the last frame, so the only correct
		// moment to ask is after the stream has ended. Reading it earlier would
		// record an empty channel for every request.
		recordStreamSuccess := func(model config.ModelConfig, upstreamChannel, upstreamModel string, keyHint string, firstByte time.Time) {
			cancelAttempt()
			complete(clientCtx, nil)
			latency := time.Since(streamStart)
			h.metrics.RecordSuccess(model.ModelID, latency)
			h.logger.Info("streaming completed",
				"request_id", requestID, "record_id", recordID,
				"model", model.ModelID,
				"upstream_model", upstreamModel,
				"latency", latency,
				"input_tokens", rw.usage.inputTokens,
				"output_tokens", rw.usage.outputTokens,
				"cache_read_input_tokens", rw.usage.cacheReadInputTokens,
				"cache_creation_input_tokens", rw.usage.cacheCreationInputTokens,
			)
			rec := history.RequestRecord{
				ID:                  recordID,
				Model:               model.ModelID,
				RequestedModel:      requestedModel,
				Provider:            providerName,
				Scenario:            string(scenario),
				StartTime:           streamStart,
				Duration:            latency,
				InputTokens:         rw.usage.inputTokens,
				OutputTokens:        rw.usage.outputTokens,
				CacheReadTokens:     rw.usage.cacheReadInputTokens,
				CacheCreationTokens: rw.usage.cacheCreationInputTokens,
				ReasoningTokens:     rw.usage.reasoningTokens,
				Streaming:           true,
				Success:             true,
				Attempt:             attempt + 1,
				PeakMultiplier:      history.ProviderPeakMultiplier(providerName, model.ModelID, streamStart),
				UpstreamChannel:     upstreamChannel,
				UpstreamModel:       upstreamModel,
				KeyHint:             keyHint,
				FirstByte:           firstByteDuration(firstByte, streamStart),
				ReasoningEffort:     clientEffort,
			}
			rw.timing.applyTo(&rec)
			rec.GenerationSample = generation.Sample()
			if h.storage != nil {
				if err := h.storage.InsertRequest(rec); err != nil {
					h.logger.Warn("failed to insert request into storage", "error", err)
				}
			}
		}

		// recordStreamFailure persists an interrupted stream that produced
		// usage. The platform bills partially produced streams by the tokens
		// actually consumed, so a failure after SSE payload started leaves a
		// permanent one-sided gap unless the proxy records it too. Streams
		// without reported usage cannot be assigned synthetic token counts.
		recordStreamFailure := func(model config.ModelConfig, err error, action string, failed bool) {
			cancelAttempt()
			rw.mu.Lock()
			hasUsage := rw.usage.inputTokens != 0 || rw.usage.outputTokens != 0 ||
				rw.usage.cacheReadInputTokens != 0 || rw.usage.cacheCreationInputTokens != 0
			// A client may disconnect after receiving an upstream error frame.
			// Preserve that observed failure even if cancellation wins the read loop.
			failed = failed || rw.streamErr != nil
			// payloadWritten is the fact that separates "the client saw a broken
			// answer" from "the client hung up on its own". Once bytes are on the
			// wire, every failure here is visible to the caller as a truncated
			// response; before that, the client was still waiting and chose to
			// leave. Read under the same lock as usage so the two agree.
			payloadWritten := rw.ssePayloadWritten
			rw.mu.Unlock()
			// Count the known outcome independently of billing. Preserve the
			// existing accounting for cancellations with reported usage.
			if failed || hasUsage {
				h.metrics.RecordFailureForModel(model.ModelID)
			}
			if h.storage == nil {
				return
			}
			// A failure the client could see is recorded even when the upstream
			// reported no usage. Those are exactly the requests a reader most
			// needs to find: the answer died before producing anything, so the
			// success rate on the dashboard was reporting 100% while the journal
			// carried the failure. The row is written with details_known = 0 so
			// every average - success rate, latency, throughput - skips it; only
			// the request count and the outcome mention it.
			//
			// A cancellation the client made before any payload is not a failure
			// of anything: nothing broke, the caller simply stopped waiting.
			// Writing it would turn every interrupted prompt into a red row.
			if !hasUsage && !payloadWritten {
				h.logger.Warn(action+" streaming failed, no usage reported", "model", model.ModelID, "error", err)
				return
			}
			h.logger.Warn("recording interrupted stream", "request_id", requestID, "record_id", recordID, "error", err)
			// The same observations the success path records, read from the same
			// source. A stream that failed after producing usage was being stored
			// with these three empty, so a failed row could not say which model
			// actually ran, how long the first byte took, or what level the
			// client asked for - exactly the fields someone investigating the
			// failure needs. currentStream is the attempt's body, so it answers
			// for the attempt that failed.
			firstByte := client.FirstByteAtOf(currentStream)
			rec := history.RequestRecord{
				ID:                  recordID,
				Model:               model.ModelID,
				RequestedModel:      requestedModel,
				Provider:            providerName,
				Scenario:            string(scenario),
				StartTime:           streamStart,
				Duration:            time.Since(streamStart),
				InputTokens:         rw.usage.inputTokens,
				OutputTokens:        rw.usage.outputTokens,
				CacheReadTokens:     rw.usage.cacheReadInputTokens,
				CacheCreationTokens: rw.usage.cacheCreationInputTokens,
				ReasoningTokens:     rw.usage.reasoningTokens,
				Streaming:           true,
				Success:             false,
				Attempt:             attempt + 1,
				PeakMultiplier:      history.ProviderPeakMultiplier(providerName, model.ModelID, streamStart),
				UpstreamChannel:     client.UpstreamChannelOf(currentStream),
				UpstreamModel:       client.UpstreamModelOf(currentStream),
				KeyHint:             client.KeyHintOf(currentStream),
				FirstByte:           firstByteDuration(firstByte, streamStart),
				ReasoningEffort:     clientEffort,
				// Usage was never reported, so the outcome is the only observed
				// fact. Marking the row unobserved keeps it out of every average
				// that assumes a measured request: a zero-token, zero-latency row
				// counted as sampled would drag success rate and throughput down
				// with a measurement nobody made.
				UsageUnobserved: !hasUsage,
			}
			rw.timing.applyTo(&rec)
			if err := h.storage.InsertRequest(rec); err != nil {
				h.logger.Warn("failed to insert failed request into storage", "error", err)
			}
		}

		// handleStreamError checks the error from a streaming attempt and
		// decides whether to retry the next model or abort. It returns true
		// if the caller should continue (fallback to next model), or false
		// if it should return.
		handleStreamError := func(err error, model config.ModelConfig, action string) bool {
			cancelAttempt()
			lastStreamErr = err
			if complete(clientCtx, err) {
				blockedProviders[providerName] = true
			}
			if clientCtx.Err() != nil {
				h.logger.Debug("client disconnected during " + action + " stream")
				recordStreamFailure(model, err, action, false)
				return false // abort
			}
			if err == transformer.ErrStreamIdle {
				h.logger.Warn("upstream "+action+" stream idle, trying next model",
					"model", model.ModelID, "idle_timeout", idleTimeout)
				if rw.ssePayloadWritten {
					h.sendStreamError(rw, "stream idle after SSE payload started")
					recordStreamFailure(model, err, action, true)
					return false // abort
				}
				return true // continue to next model
			}
			if err == transformer.ErrEmptyStream {
				h.logger.Warn("upstream "+action+" stream empty, trying next model",
					"model", model.ModelID)
				if rw.ssePayloadWritten {
					h.sendStreamError(rw, "empty stream after SSE payload started")
					recordStreamFailure(model, err, action, true)
					return false // abort
				}
				return true // continue to next model
			}
			// A rejected request is not a failing model. Every model in the chain
			// would refuse it identically, so the answer goes back to the client
			// now instead of costing quota on each remaining platform.
			if router.IsNonRetryableRequestError(err) {
				h.logger.Warn("request rejected as invalid, not falling back",
					"model", model.ModelID, "error", err)
				lastStreamErr = err
				if rw.ssePayloadWritten {
					h.sendStreamError(rw, streamFailureMessage(err))
					recordStreamFailure(model, err, action, true)
					return false
				}
				return false
			}
			h.logger.Warn(action+" streaming failed", "model", model.ModelID, "error", err)
			if rw.ssePayloadWritten {
				h.sendStreamError(rw, "all upstream models failed after SSE payload started")
				recordStreamFailure(model, err, action, true)
				return false // abort — cannot fallback after SSE payload started
			}
			return true // continue to next model
		}
		finishStream := func(model config.ModelConfig, action string) bool {
			if err := rw.finish(); err != nil {
				return handleStreamError(err, model, action)
			}
			recordStreamSuccess(model, client.UpstreamChannelOf(currentStream), client.UpstreamModelOf(currentStream), client.KeyHintOf(currentStream), client.FirstByteAtOf(currentStream))
			return false
		}

		prov, ok := h.providerRegistry.Get(providerName)
		if !ok {
			cancelAttempt()
			lastStreamErr = fmt.Errorf("provider %q is not registered", providerName)
			complete(clientCtx, lastStreamErr)
			continue
		}
		wireFormat := core.ModelWireFormat(prov, model)
		attemptCtx, generation = metrics.WithGenerationObservation(attemptCtx, streamStart, wireFormat)
		streamBody, err := prov.Stream(attemptCtx, anthropicReq, model)
		if err != nil {
			cancelAttempt()
			if complete(clientCtx, err) {
				blockedProviders[providerName] = true
			}
			if clientCtx.Err() != nil {
				h.logger.Debug("client disconnected during upstream request")
				return
			}
			// Streaming cannot retry once the SSE head is out, so a
			// deterministic refusal is the client's answer. Keeping it
			// lets the terminal error below name the cause instead of a
			// generic "all streaming models failed".
			lastStreamErr = err
			h.logger.Warn("streaming request failed via provider", "model", model.ModelID, "provider", model.Provider, "error", err)
			continue
		}

		// Bind body read to attemptCtx so streaming_timeout_ms aborts mid-stream.
		streamReader := transformer.NewCtxReadCloser(attemptCtx, streamBody)
		currentStream = streamReader

		if wireFormat == core.WireFormatAnthropic {
			atomic.StoreInt32(&heartbeatPaused, 1)
		}
		// The model to name in message_start. The provider peeked the first
		// upstream chunk, so this is what the gateway says it is running - not
		// the id we asked for. They differ whenever the gateway answers one id
		// by running a virtual model behind its own fallback chain, and naming
		// the request id there would describe a model that never ran.
		servedModel := client.ServedModelOf(streamReader)
		if servedModel == "" {
			servedModel = model.ModelID
		}
		errProxy := h.streamProxy.ProxyStream(rw, streamReader, wireFormat, servedModel, attemptCtx, idleTimeout, cancelAttempt)
		if wireFormat == core.WireFormatAnthropic {
			atomic.StoreInt32(&heartbeatPaused, 0)
		}
		if errProxy != nil {
			// The body reader uses a cancellation sentinel; retain the attempt's
			// deadline identity for health settlement without blaming parent cancellation.
			if errors.Is(errProxy, transformer.ErrStreamReadCanceled) && attemptCtx.Err() == context.DeadlineExceeded {
				errProxy = fmt.Errorf("streaming timeout (%v) exceeded: %w: %w", timeout, attemptCtx.Err(), errProxy)
			}
			if errProxy == transformer.ErrClientDisconnected {
				if clientCtx.Err() != nil {
					complete(clientCtx, errProxy)
					h.logger.Debug("client disconnected during stream")
					recordStreamFailure(model, errProxy, wireFormat.String(), false)
					return
				}
				errProxy = fmt.Errorf("streaming timeout (%v) exceeded", timeout)
			}
			if !handleStreamError(errProxy, model, wireFormat.String()) {
				return
			}
			continue
		}

		// OpenRouter's former Chat Completions path accepted completed empty
		// replies. Moving its transport must not add this provider heuristic.
		if providerName != client.ProviderOpenRouter && isLowValueResponse(scenario, rw.getOutputTokens(), rw.hasContent()) {
			h.logger.Warn("upstream returned low-value response, triggering fallback",
				"model", model.ModelID, "provider", model.Provider,
				"scenario", scenario, "output_tokens", rw.getOutputTokens())
			if !handleStreamError(transformer.ErrEmptyStream, model, wireFormat.String()) {
				return
			}
			continue
		}

		if finishStream(model, wireFormat.String()) {
			continue
		}
		return
	}

	h.metrics.RecordFailure()
	if rw.ssePayloadWritten {
		// SSE payload was already sent — do not attempt further writes
		// beyond the error event.  The client has a partial stream.
		return
	}
	if !rw.wroteHeader {
		h.sendError(w, http.StatusBadGateway, "all streaming models failed", lastStreamErr)
	} else {
		h.sendStreamError(rw, streamFailureMessage(lastStreamErr))
	}
}

// streamFailureMessage names the cause of an exhausted streaming chain for the
// SSE error event. The event carries a message and no status, so the upstream
// text is the only detail the client can get.
func streamFailureMessage(err error) string {
	if err == nil {
		return "all upstream models failed"
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		if body := strings.TrimSpace(apiErr.Body); body != "" {
			return truncateError(body)
		}
		return apiErr.Error()
	}
	return truncateError(err.Error())
}

// sendStreamError sends an error event in the SSE stream.
func (h *MessagesHandler) sendStreamError(w http.ResponseWriter, message string) {
	h.logger.Error("sending stream error", "message", message)

	errorEvent := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "api_error",
			"message": message,
		},
	}

	data, _ := json.Marshal(errorEvent)
	if _, err := fmt.Fprintf(w, "event: error\ndata: %s\n\n", string(data)); err != nil {
		h.logger.Debug("failed to write stream error event", "error", err, "message", message)
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// handleNonStreaming handles a non-streaming request with fallback.
func decodeMessageUsage(responseBody []byte) types.Usage {
	var response types.MessageResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return types.Usage{}
	}
	return response.Usage
}

func (h *MessagesHandler) handleNonStreaming(
	w http.ResponseWriter,
	r *http.Request,
	anthropicReq *types.MessageRequest,
	modelChain []config.ModelConfig,
	rawBody json.RawMessage,
	scenario router.Scenario,
	requestID string,
) {
	ctx := h.metrics.WithUpstreamCounting(r.Context())
	startTime := time.Now()
	// See handleStreaming: kept for the same reason, from the same source.
	requestedModel := anthropicReq.Model
	recordID := uuid.NewString()

	// Set by the attempt that succeeds, then stored. Empty for every platform
	// whose upstream does not name a channel.
	var upstreamChannel, upstreamModel, keyHint string
	clientEffort := history.RequestedEffort(anthropicReq)

	result, responseBody, err := h.fallbackHandler.ExecuteWithFallback(
		ctx,
		modelChain,
		func(ctx context.Context, model config.ModelConfig, attempt int) ([]byte, error) {
			attemptStart := time.Now()
			timeout := h.client.RequestTimeout(model)
			attemptCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			prov, ok := h.providerRegistry.Get(client.Provider(model))
			if !ok {
				return nil, fmt.Errorf("provider %q is not registered", client.Provider(model))
			}
			execResult, execErr := prov.Execute(attemptCtx, anthropicReq, model)
			if execErr != nil {
				// Record each reported failure before fallback can replace it with
				// another execution. Missing usage is not an observed zero.
				var failure *core.ResponsesStatusError
				if errors.As(execErr, &failure) && failure.Usage != nil && h.storage != nil {
					usage := failure.Usage
					rec := history.RequestRecord{
						ID: uuid.NewString(), Model: model.ModelID, RequestedModel: requestedModel,
						Provider: client.Provider(model), Scenario: string(scenario),
						StartTime: attemptStart, Duration: time.Since(attemptStart),
						InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
						CacheReadTokens: usage.CacheReadInputTokens, CacheCreationTokens: usage.CacheCreationInputTokens,
						ReasoningTokens: usage.ReasoningTokens,
						Success:         false, ErrorMsg: execErr.Error(), Attempt: attempt,
					}
					if err := h.storage.InsertRequest(rec); err != nil {
						h.logger.Warn("failed to insert request into storage", "error", err)
					}
				}
				return nil, execErr
			}
			// The upstream body has been drained by Execute, so the channel the
			// gateway named in its last frame is available now.
			upstreamChannel = execResult.UpstreamChannel
			upstreamModel = execResult.UpstreamModel
			keyHint = execResult.KeyHint
			return execResult.Body, nil
		},
	)

	if err != nil {
		// An attempt's deadline can expire while the client is still waiting.
		// Only the parent request decides whether there is no response to send.
		if ctx.Err() != nil {
			h.logger.Info("request context canceled during non-streaming fallback", "error", err)
			return
		}
		h.metrics.RecordFailureForModel(result.ModelID)
		h.sendError(w, http.StatusBadGateway, "all models failed", err)
		return
	}

	usage := decodeMessageUsage(responseBody)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	n, writeErr := w.Write(responseBody)
	if writeErr == nil && n != len(responseBody) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		if finisher, ok := w.(interface{ Finish() error }); ok {
			writeErr = finisher.Finish()
		}
	}
	latency := time.Since(startTime)
	if writeErr != nil {
		h.metrics.RecordFailureForModel(result.ModelID)
		h.logger.Warn("response delivery failed", "model", result.ModelID, "request_id", requestID, "record_id", recordID, "error", writeErr)
	} else {
		h.metrics.RecordSuccess(result.ModelID, latency)
		h.logger.Info("request completed", "model", result.ModelID, "request_id", requestID, "record_id", recordID, "attempts", result.Attempted, "latency", latency)
	}

	rec := history.RequestRecord{
		ID:                  recordID,
		Model:               result.ModelID,
		RequestedModel:      requestedModel,
		Provider:            result.Provider,
		Scenario:            string(scenario),
		StartTime:           startTime,
		Duration:            latency,
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		CacheReadTokens:     usage.CacheReadInputTokens,
		CacheCreationTokens: usage.CacheCreationInputTokens,
		ReasoningTokens:     usage.ReasoningTokens,
		Streaming:           false,
		Success:             writeErr == nil,
		Attempt:             result.Attempted,
		PeakMultiplier:      history.ProviderPeakMultiplier(result.Provider, result.ModelID, startTime),
		UpstreamChannel:     upstreamChannel,
		UpstreamModel:       upstreamModel,
		KeyHint:             keyHint,
		ReasoningEffort:     clientEffort,
	}
	if h.storage != nil {
		if err := h.storage.InsertRequest(rec); err != nil {
			h.logger.Warn("failed to insert request into storage", "error", err)
		}
	}
}

// firstByteDuration is the gap from the request's start to the upstream's first
// byte, or zero when that moment was never observed. A zero is stored as "not
// measured" rather than as a duration, so a provider that does not report it
// cannot look like an instant one.
func firstByteDuration(at, start time.Time) time.Duration {
	if at.IsZero() || start.IsZero() || at.Before(start) {
		return 0
	}
	return at.Sub(start)
}

// extractTextFromBlocks extracts plain text from Anthropic content blocks.
func extractTextFromBlocks(blocks []types.ContentBlock) string {
	var content string
	for _, block := range blocks {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			content += fmt.Sprintf("[Tool Use: %s]", block.Name)
		case "tool_result":
			content += block.TextContent()
		case "thinking":
			// Skip thinking blocks for text extraction
		case "image":
			content += "[Image]"
		}
	}
	return content
}

func blocksHaveImage(blocks []types.ContentBlock) bool {
	for _, block := range blocks {
		if block.Type == "image" && block.Source != nil {
			return true
		}
	}
	return false
}

func imageHashesFromBlocks(blocks []types.ContentBlock) []string {
	var hashes []string
	for _, block := range blocks {
		if block.Type != "image" || block.Source == nil {
			continue
		}
		source := block.Source.Type + "\x00" + block.Source.MediaType + "\x00" + block.Source.Data + "\x00" + block.Source.URL
		sum := sha256.Sum256([]byte(source))
		hashes = append(hashes, hex.EncodeToString(sum[:]))
	}
	return hashes
}

// maxForwardedErrorBody bounds what a client sees of an upstream error body.
// The body is untrusted and can hold anything the platform chose to include, so
// only enough to identify the refusal is passed on.
const maxForwardedErrorBody = 4096

// clientError maps an internal failure to the status and message the client
// should see. Upstream refusals keep their own status and body: the proxy is a
// gateway for several platforms, and a client can only react to a failure the
// platform actually named. Rewriting every upstream error into a gateway status
// both hides the cause and lets an intermediary replace the body - Cloudflare
// substitutes its own page for a 502, so the reason never arrived at all.
func clientError(statusCode int, message string, err error) (int, string) {
	// Retained upstream refusal: this is the platform's own answer and already
	// maps to an Anthropic status, so pass it through unchanged.
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		if body := strings.TrimSpace(apiErr.Body); body != "" {
			return apiErr.StatusCode, truncateError(body)
		}
		return apiErr.StatusCode, apiErr.Error()
	}

	// The chain was scoped to the active platform and came up empty. That is a
	// configuration fault on this side, not an upstream failure.
	if errors.Is(err, router.ErrWithinSiteNoTarget) {
		return http.StatusServiceUnavailable, err.Error()
	}

	return statusCode, message
}

// truncateError returns the error text up to maxForwardedErrorBody bytes, cut on
// a rune boundary so the result stays valid UTF-8.
func truncateError(s string) string {
	if len(s) <= maxForwardedErrorBody {
		return s
	}
	cut := maxForwardedErrorBody
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "… (truncated)"
}

// sendError sends an error response in Anthropic format.
func (h *MessagesHandler) sendError(w http.ResponseWriter, statusCode int, message string, err error) {
	clientStatus, clientMessage := clientError(statusCode, message, err)

	h.logger.Error("request error",
		"status", clientStatus,
		"message", clientMessage,
		"error", err,
	)

	if rw, ok := w.(*responseWriter); ok && rw.headerWritten() {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(clientStatus)

	errorResp := transformer.TransformErrorResponse(clientStatus, clientMessage)
	_ = json.NewEncoder(w).Encode(errorResp)
}
