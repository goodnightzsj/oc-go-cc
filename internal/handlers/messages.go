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
	client              *client.OpenCodeClient // kept for backward compat during migration
	providerRegistry    *core.ProviderRegistry // new: provider dispatch
	modelRouter         *router.ModelRouter
	fallbackHandler     *router.FallbackHandler
	streamProxy         *StreamProxy // new: SSE proxy by wire format
	requestTransformer  *transformer.RequestTransformer
	responseTransformer *transformer.ResponseTransformer
	streamHandler       *transformer.StreamHandler
	tokenCounter        *token.Counter
	logger              *slog.Logger
	rateLimiter         *middleware.RateLimiter
	requestIDGen        *middleware.RequestIDGenerator
	metrics             *metrics.Metrics
	captureLogger       *debug.CaptureLogger
	storage             StorageWriter // optional: SQLite persistence for requests/latency
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
	usage             struct {
		inputTokens              int
		outputTokens             int
		cacheReadInputTokens     int
		cacheCreationInputTokens int
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
	if len(b) > 0 {
		w.ssePayloadWritten = true
		if err := w.observeSSE(b); err != nil {
			w.streamErr = err
			return 0, err
		}
	}
	return w.ResponseWriter.Write(b)
}

// observeSSE reads complete events without changing forwarded bytes. Native
// streams may split any JSON token between writes and report input/cache usage
// only in message_start; message_delta updates only fields actually present.
// Write holds mu while calling this method.
func (w *responseWriter) observeSSE(b []byte) error {
	w.sseBuffer = append(w.sseBuffer, b...)
	for {
		i := bytes.IndexByte(w.sseBuffer, '\n')
		if i < 0 {
			return nil
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
				if err := w.observeSSEEvent(); err != nil {
					return err
				}
			}
			w.sseData = w.sseData[:0]
			w.sseEvent = ""
		}
	}
}

func (w *responseWriter) observeSSEEvent() error {
	var event struct {
		Type    string         `json:"type"`
		Usage   map[string]int `json:"usage"`
		Message struct {
			Usage map[string]int `json:"usage"`
		} `json:"message"`
	}
	data := w.sseData
	w.sseData = nil
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("invalid upstream SSE event: %w", err)
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
	for field, value := range usage {
		switch field {
		case "input_tokens":
			w.usage.inputTokens = value
		case "output_tokens":
			w.usage.outputTokens = value
		case "cache_read_input_tokens":
			w.usage.cacheReadInputTokens = value
		case "cache_creation_input_tokens":
			w.usage.cacheCreationInputTokens = value
		}
	}
	return nil
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

// SetPartialUsage records usage surfaced by the transformer from a
// mid-stream OpenAI chunk (every chunk carries cumulative usage, so the last
// value seen before an upstream failure is the exact count the platform
// bills for a partially produced stream). Called from the transform
// goroutine, the same one that calls observeSSE via Write.
func (w *responseWriter) SetPartialUsage(in, out, cacheRead, cacheCreate int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.usage.inputTokens = in
	w.usage.outputTokens = out
	w.usage.cacheReadInputTokens = cacheRead
	w.usage.cacheCreationInputTokens = cacheCreate
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
		client:              openCodeClient,
		providerRegistry:    providerRegistry,
		modelRouter:         modelRouter,
		fallbackHandler:     fallbackHandler,
		streamProxy:         NewStreamProxy(),
		requestTransformer:  transformer.NewRequestTransformer(),
		responseTransformer: transformer.NewResponseTransformer(),
		streamHandler:       transformer.NewStreamHandler(),
		tokenCounter:        tokenCounter,
		logger:              slog.Default(),
		rateLimiter:         middleware.NewRateLimiter(100, time.Minute),
		requestIDGen:        middleware.NewRequestIDGenerator(),
		metrics:             metrics,
		captureLogger:       captureLogger,
		storage:             storage,
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
	if published, ok := h.modelRouter.PublishedByActiveSite(ctx, requestedModel); ok {
		primary := config.ModelConfig{Provider: h.modelRouter.ActiveSite(), ModelID: published}
		primary = config.ResolveModelConfig(primary)
		// The scenario chain stays as a safety net, but only within the active
		// site: falling back to another platform is what the scope exists to
		// prevent.
		fallbacks, _ := h.routeOnce(routerMessages, tokenCount, "", isStreaming)
		net := router.RestrictToActiveSite(h.modelRouter.ActiveSite(), fallbacks.GetModelChain())
		result := router.RouteResult{Primary: primary, Scenario: router.ScenarioOverride}
		return appendUniqueModels([]config.ModelConfig{primary}, net), result, nil
	}

	if requestedModel != "" {
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

	// The active site scopes the chain rather than reordering it. An empty
	// result is the operator's configuration not covering the selected site,
	// and it is reported as such instead of routing to a platform they did not
	// choose.
	if active := h.modelRouter.ActiveSite(); active != "" {
		decision.Models = router.RestrictToActiveSite(active, decision.Models)
		if len(decision.Models) == 0 {
			return nil, result, fmt.Errorf("active site %q has no routing target for this request; configure a model for it or switch the site", active)
		}
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
	clientCtx := r.Context()

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
	blockedProviders := make(map[string]bool)

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

		h.logger.Info("attempting streaming model", "model", model.ModelID, "provider", model.Provider)

		// Upstream context carries the streaming timeout configured for the model.
		timeout := h.client.StreamingTimeout(model)
		attemptCtx, cancelAttempt := context.WithTimeout(clientCtx, timeout)
		idleTimeout := h.client.StreamIdleTimeout(model)

		// recordStreamSuccess records a successful stream completion and
		// marks the model attempt as done.
		recordStreamSuccess := func(model config.ModelConfig) {
			cancelAttempt()
			latency := time.Since(streamStart)
			h.metrics.RecordSuccess(model.ModelID, latency)
			h.logger.Info("streaming completed",
				"model", model.ModelID,
				"latency", latency,
				"input_tokens", rw.usage.inputTokens,
				"output_tokens", rw.usage.outputTokens,
				"cache_read_input_tokens", rw.usage.cacheReadInputTokens,
				"cache_creation_input_tokens", rw.usage.cacheCreationInputTokens,
			)
			rec := history.RequestRecord{
				ID:                  requestID,
				Model:               model.ModelID,
				Provider:            providerName,
				Scenario:            string(scenario),
				StartTime:           streamStart,
				Duration:            latency,
				InputTokens:         rw.usage.inputTokens,
				OutputTokens:        rw.usage.outputTokens,
				CacheReadTokens:     rw.usage.cacheReadInputTokens,
				CacheCreationTokens: rw.usage.cacheCreationInputTokens,
				Streaming:           true,
				Success:             true,
				Attempt:             attempt + 1,
				PeakMultiplier:      history.ProviderPeakMultiplier(providerName, model.ModelID, streamStart),
			}
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
		recordStreamFailure := func(model config.ModelConfig, err error, action string) {
			cancelAttempt()
			if rw.usage.inputTokens == 0 && rw.usage.outputTokens == 0 &&
				rw.usage.cacheReadInputTokens == 0 && rw.usage.cacheCreationInputTokens == 0 {
				h.logger.Warn(action+" streaming failed, no usage reported", "model", model.ModelID, "error", err)
				return
			}
			h.metrics.RecordFailureForModel(model.ModelID)
			if h.storage == nil {
				return
			}
			rec := history.RequestRecord{
				ID:                  requestID,
				Model:               model.ModelID,
				Provider:            providerName,
				Scenario:            string(scenario),
				StartTime:           streamStart,
				Duration:            time.Since(streamStart),
				InputTokens:         rw.usage.inputTokens,
				OutputTokens:        rw.usage.outputTokens,
				CacheReadTokens:     rw.usage.cacheReadInputTokens,
				CacheCreationTokens: rw.usage.cacheCreationInputTokens,
				Streaming:           true,
				Success:             false,
				Attempt:             attempt + 1,
				PeakMultiplier:      history.ProviderPeakMultiplier(providerName, model.ModelID, streamStart),
			}
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
			if clientCtx.Err() != nil {
				h.logger.Debug("client disconnected during " + action + " stream")
				recordStreamFailure(model, err, action)
				return false // abort
			}
			if err == transformer.ErrStreamIdle {
				h.logger.Warn("upstream "+action+" stream idle, trying next model",
					"model", model.ModelID, "idle_timeout", idleTimeout)
				if rw.ssePayloadWritten {
					h.sendStreamError(rw, "stream idle after SSE payload started")
					recordStreamFailure(model, err, action)
					return false // abort
				}
				return true // continue to next model
			}
			if err == transformer.ErrEmptyStream {
				h.logger.Warn("upstream "+action+" stream empty, trying next model",
					"model", model.ModelID)
				if rw.ssePayloadWritten {
					h.sendStreamError(rw, "empty stream after SSE payload started")
					recordStreamFailure(model, err, action)
					return false // abort
				}
				return true // continue to next model
			}
			h.logger.Warn(action+" streaming failed", "model", model.ModelID, "error", err)
			if rw.ssePayloadWritten {
				h.sendStreamError(rw, "all upstream models failed after SSE payload started")
				recordStreamFailure(model, err, action)
				return false // abort — cannot fallback after SSE payload started
			}
			return true // continue to next model
		}
		finishStream := func(model config.ModelConfig, action string) bool {
			if err := rw.finish(); err != nil {
				return handleStreamError(err, model, action)
			}
			recordStreamSuccess(model)
			return false
		}

		// Try new provider-based dispatch first.
		if h.providerRegistry != nil {
			if prov, ok := h.providerRegistry.Get(client.Provider(model)); ok {
				streamBody, err := prov.Stream(attemptCtx, anthropicReq, model)
				if err != nil {
					cancelAttempt()
					if clientCtx.Err() != nil {
						h.logger.Debug("client disconnected during upstream request")
						return
					}
					if router.IsUsageLimitError(err) {
						blockedProviders[providerName] = true
					}
					h.logger.Warn("streaming request failed via provider", "model", model.ModelID, "provider", model.Provider, "error", err)
					continue
				}

				// Bind body read to attemptCtx so streaming_timeout_ms aborts mid-stream.
				streamReader := transformer.NewCtxReadCloser(attemptCtx, streamBody)

				wireFormat := core.ModelWireFormat(prov, model)
				if wireFormat == core.WireFormatAnthropic {
					atomic.StoreInt32(&heartbeatPaused, 1)
				}
				errProxy := h.streamProxy.ProxyStream(rw, streamReader, wireFormat, model.ModelID, attemptCtx, idleTimeout, cancelAttempt)
				if wireFormat == core.WireFormatAnthropic {
					atomic.StoreInt32(&heartbeatPaused, 0)
				}
				if errProxy != nil {
					if errProxy == transformer.ErrClientDisconnected {
						if clientCtx.Err() != nil {
							h.logger.Debug("client disconnected during stream")
							recordStreamFailure(model, errProxy, wireFormat.String())
							return
						}
						errProxy = fmt.Errorf("streaming timeout (%v) exceeded", timeout)
					}
					if !handleStreamError(errProxy, model, wireFormat.String()) {
						return
					}
					continue
				}

				if isLowValueResponse(scenario, rw.getOutputTokens(), rw.hasContent()) {
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
		}

		// Providers without a registry entry (e.g. OpenRouter) are plain
		// OpenAI-compatible upstreams.
		h.logger.Warn("provider not in registry, using OpenAI-compatible path",
			"provider", model.Provider, "model", model.ModelID)

		openaiReq, err := h.requestTransformer.TransformRequest(anthropicReq, model)
		if err != nil {
			cancelAttempt()
			h.logger.Warn("request transform failed", "model", model.ModelID, "error", err)
			continue
		}

		streamBody, err := h.client.GetStreamingBody(attemptCtx, model.ModelID, openaiReq, model)
		if err != nil {
			cancelAttempt()
			if clientCtx.Err() != nil {
				h.logger.Debug("client disconnected during upstream request")
				return
			}
			h.logger.Warn("streaming request failed", "model", model.ModelID, "error", err)
			continue
		}

		// Bind body read to attemptCtx so streaming_timeout_ms aborts mid-stream.
		streamReader := transformer.NewCtxReadCloser(attemptCtx, streamBody)

		if err := h.streamHandler.ProxyStream(rw, streamReader, model.ModelID, attemptCtx, idleTimeout, cancelAttempt); err != nil {
			if err == transformer.ErrClientDisconnected {
				if clientCtx.Err() != nil {
					h.logger.Debug("client disconnected during stream")
					recordStreamFailure(model, err, "openai")
					return
				}
				err = fmt.Errorf("streaming timeout (%v) exceeded", timeout)
			}
			if !handleStreamError(err, model, "openai") {
				return
			}
			continue
		}

		if finishStream(model, "openai") {
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
		h.sendError(w, http.StatusBadGateway, "all streaming models failed", nil)
	} else {
		h.sendStreamError(rw, "all upstream models failed")
	}
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
	ctx := r.Context()
	startTime := time.Now()

	result, responseBody, err := h.fallbackHandler.ExecuteWithFallback(
		ctx,
		modelChain,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			timeout := h.client.RequestTimeout(model)
			attemptCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// Try new provider-based dispatch first.
			if h.providerRegistry != nil {
				if prov, ok := h.providerRegistry.Get(client.Provider(model)); ok {
					execResult, execErr := prov.Execute(attemptCtx, anthropicReq, model)
					if execErr != nil {
						return nil, execErr
					}
					return execResult.Body, nil
				}
			}

			// Providers without a registry entry (e.g. OpenRouter) are plain
			// OpenAI-compatible upstreams.
			h.logger.Warn("provider not in registry, using OpenAI-compatible path",
				"provider", model.Provider, "model", model.ModelID)

			return h.executeOpenAIRequest(attemptCtx, anthropicReq, model)
		},
	)

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
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
		h.logger.Warn("response delivery failed", "model", result.ModelID, "request_id", requestID, "error", writeErr)
	} else {
		h.metrics.RecordSuccess(result.ModelID, latency)
		h.logger.Info("request completed", "model", result.ModelID, "attempts", result.Attempted, "latency", latency)
	}

	rec := history.RequestRecord{
		ID:                  requestID,
		Model:               result.ModelID,
		Provider:            result.Provider,
		Scenario:            string(scenario),
		StartTime:           startTime,
		Duration:            latency,
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		CacheReadTokens:     usage.CacheReadInputTokens,
		CacheCreationTokens: usage.CacheCreationInputTokens,
		Streaming:           false,
		Success:             writeErr == nil,
		Attempt:             result.Attempted,
		PeakMultiplier:      history.ProviderPeakMultiplier(result.Provider, result.ModelID, startTime),
	}
	if h.storage != nil {
		if err := h.storage.InsertRequest(rec); err != nil {
			h.logger.Warn("failed to insert request into storage", "error", err)
		}
	}
}

// executeOpenAIRequest executes a request to the OpenAI endpoint with transformation.
func (h *MessagesHandler) executeOpenAIRequest(
	ctx context.Context,
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
) ([]byte, error) {
	openaiReq, err := h.requestTransformer.TransformRequest(anthropicReq, model)
	if err != nil {
		return nil, fmt.Errorf("request transform failed: %w", err)
	}

	resp, err := h.client.ChatCompletionNonStreaming(ctx, model.ModelID, openaiReq, model)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	anthropicResp, err := h.responseTransformer.TransformResponse(resp, model.ModelID)
	if err != nil {
		return nil, fmt.Errorf("response transform failed: %w", err)
	}

	return json.Marshal(anthropicResp)
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

// sendError sends an error response in Anthropic format.
func (h *MessagesHandler) sendError(w http.ResponseWriter, statusCode int, message string, err error) {
	h.logger.Error("request error",
		"status", statusCode,
		"message", message,
		"error", err,
	)

	if rw, ok := w.(*responseWriter); ok && rw.headerWritten() {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := transformer.TransformErrorResponse(statusCode, message)
	_ = json.NewEncoder(w).Encode(errorResp)
}
