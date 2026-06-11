// Package handlers contains HTTP request handlers for API endpoints.
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"oc-go-cc/internal/client"
	"oc-go-cc/internal/config"
	"oc-go-cc/internal/lifecycle"
	"oc-go-cc/internal/metrics"
	"oc-go-cc/internal/middleware"
	"oc-go-cc/internal/router"
	"oc-go-cc/internal/token"
	"oc-go-cc/internal/transformer"
	"oc-go-cc/pkg/types"
)

// MessagesHandler handles /v1/messages requests.
type MessagesHandler struct {
	atomic              *config.AtomicConfig
	client              *client.OpenCodeClient
	modelRouter         *router.ModelRouter
	fallbackHandler     *router.FallbackHandler
	requestTransformer  *transformer.RequestTransformer
	responseTransformer *transformer.ResponseTransformer
	streamHandler       *transformer.StreamHandler
	tokenCounter        *token.Counter
	logger              *slog.Logger
	rateLimiter         *middleware.RateLimiter
	requestDedup        *middleware.RequestDeduplicator
	requestIDGen        *middleware.RequestIDGenerator
	metrics             *metrics.Metrics
	lifecycle           *lifecycle.State
}

// responseWriter wraps http.ResponseWriter to track if headers were written.
type responseWriter struct {
	http.ResponseWriter
	mu          sync.Mutex
	wroteHeader bool
	statusCode  int
	captureBody bool
	capture     bytes.Buffer
}

func (w *responseWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writeHeaderLocked(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.wroteHeader {
		w.writeHeaderLocked(http.StatusOK)
	}
	if w.captureBody {
		_, _ = w.capture.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// Flush implements http.Flusher for SSE streaming support.
func (w *responseWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *responseWriter) EnableCapture() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.captureBody = true
}

func (w *responseWriter) StatusCode() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.statusCode
}

func (w *responseWriter) CapturedBody() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.capture.String()
}

func (w *responseWriter) writeHeaderLocked(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.statusCode = code
		w.ResponseWriter.WriteHeader(code)
	}
}

// NewMessagesHandler creates a new messages handler.
func NewMessagesHandler(
	atomic *config.AtomicConfig,
	openCodeClient *client.OpenCodeClient,
	modelRouter *router.ModelRouter,
	fallbackHandler *router.FallbackHandler,
	tokenCounter *token.Counter,
	metrics *metrics.Metrics,
	lifecycleState *lifecycle.State,
) *MessagesHandler {
	return &MessagesHandler{
		atomic:              atomic,
		client:              openCodeClient,
		modelRouter:         modelRouter,
		fallbackHandler:     fallbackHandler,
		requestTransformer:  transformer.NewRequestTransformer(),
		responseTransformer: transformer.NewResponseTransformer(),
		streamHandler:       transformer.NewStreamHandler(),
		tokenCounter:        tokenCounter,
		logger:              slog.Default(),
		rateLimiter:         middleware.NewRateLimiter(100, time.Minute),
		requestDedup:        middleware.NewRequestDeduplicator(500 * time.Millisecond),
		requestIDGen:        middleware.NewRequestIDGenerator(),
		metrics:             metrics,
		lifecycle:           lifecycleState,
	}
}

// HandleMessages handles POST /v1/messages.
func (h *MessagesHandler) HandleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate or get request ID for correlation
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = h.requestIDGen.Generate()
	}
	w.Header().Set("X-Request-ID", requestID)
	requestLogger := h.logger.With("request_id", requestID)

	if h.lifecycle != nil && h.lifecycle.IsDraining() {
		requestLogger.Warn("rejecting request while server is draining",
			"active_requests", h.lifecycle.ActiveRequests(),
		)
		w.Header().Set("Retry-After", "5")
		h.sendError(requestLogger, w, http.StatusServiceUnavailable, "server is restarting, please retry", nil)
		return
	}

	// Rate limiting
	clientIP := middleware.GetClientIP(r)
	if !h.rateLimiter.Allow(clientIP) {
		h.metrics.RecordRateLimited()
		requestLogger.Warn("rate limited", "client", clientIP)
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	// Read the raw request body for debug logging
	var rawBody json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
		h.sendError(requestLogger, w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	h.logInboundRequest(requestLogger, r, clientIP, rawBody)

	// Deduplicate - skip duplicate requests
	if _, ok := h.requestDedup.TryAcquire(rawBody); !ok {
		h.metrics.RecordDeduplicated()
		requestLogger.Info("duplicate request skipped")
		return
	}

	// Parse into Anthropic request
	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		h.sendError(requestLogger, w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// Validate request
	if err := anthropicReq.Validate(); err != nil {
		h.sendError(requestLogger, w, http.StatusBadRequest, err.Error(), nil)
		return
	}

	// Record metrics
	isStreaming := anthropicReq.Stream != nil && *anthropicReq.Stream
	h.metrics.RecordRequest(isStreaming)

	requestLogger.Info("received request",
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

	for _, msg := range anthropicReq.Messages {
		blocks := msg.ContentBlocks()
		content := extractTextFromBlocks(blocks)
		mc := router.MessageContent{
			Role:    msg.Role,
			Content: content,
		}
		routerMessages = append(routerMessages, mc)
		tokenMessages = append(tokenMessages, token.MessageContent{
			Role:    msg.Role,
			Content: content,
		})
	}

	// Count tokens.
	tokenCount, err := h.tokenCounter.CountMessages(systemText, tokenMessages)
	if err != nil {
		h.logger.Warn("failed to count tokens", "error", err)
		tokenCount = 0
	}

	// Route to appropriate model and build fallback chain.
	modelChain, routeResult, err := h.buildModelChain(anthropicReq.Model, routerMessages, tokenCount, isStreaming)
	if err != nil {
		h.sendError(requestLogger, w, http.StatusInternalServerError, "routing failed", err)
		return
	}

	requestLogger.Info("routing request",
		"scenario", routeResult.Scenario,
		"model", routeResult.Primary.ModelID,
		"provider", routeResult.Primary.Provider,
		"tokens", tokenCount,
	)

	if isStreaming {
		// Streaming: use ProxyStream for real-time SSE transformation
		h.handleStreaming(w, r, &anthropicReq, modelChain, rawBody, requestLogger)
	} else {
		// Non-streaming: execute with fallback and return full response
		h.handleNonStreaming(w, r, &anthropicReq, modelChain, rawBody, requestLogger)
	}
}

// buildModelChain resolves the request to a model chain (primary + fallbacks),
// honoring model_overrides (with a deduplicated scenario safety-net) and
// respecting the streaming-scenario-routing toggle.
//
// Precedence:
//  1. If requestedModel matches an entry in model_overrides, use that as the
//     primary and append the scenario chain as a deduplicated safety net.
//  2. Otherwise, fall through to scenario-based routing via routeOnce.
func (h *MessagesHandler) buildModelChain(
	requestedModel string,
	routerMessages []router.MessageContent,
	tokenCount int,
	isStreaming bool,
) ([]config.ModelConfig, router.RouteResult, error) {
	if requestedModel != "" {
		if overrideResult, ok := h.modelRouter.RouteWithOverride(requestedModel); ok {
			scenarioResult, err := h.routeOnce(routerMessages, tokenCount, "", isStreaming)
			if err != nil {
				// Override is valid; surface the scenario routing error rather
				// than silently dropping the safety net.
				return overrideResult.GetModelChain(), overrideResult, err
			}
			chain := appendUniqueModels(overrideResult.GetModelChain(), scenarioResult.GetModelChain())
			return chain, overrideResult, nil
		}
	}

	result, err := h.routeOnce(routerMessages, tokenCount, requestedModel, isStreaming)
	if err != nil {
		return nil, result, err
	}
	return result.GetModelChain(), result, nil
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
		return h.modelRouter.RouteForStreaming(routerMessages, tokenCount, requestedModel), nil
	}
	return h.modelRouter.Route(routerMessages, tokenCount, requestedModel)
}

// appendUniqueModels appends models from extra to base, skipping any model_id
// already present in base. The first occurrence of a ModelID is kept; later
// duplicates are dropped. Order of the base chain is preserved.
func appendUniqueModels(base, extra []config.ModelConfig) []config.ModelConfig {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base))
	for _, m := range base {
		seen[m.ModelID] = struct{}{}
	}
	for _, m := range extra {
		if _, ok := seen[m.ModelID]; ok {
			continue
		}
		base = append(base, m)
		seen[m.ModelID] = struct{}{}
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
	requestLogger *slog.Logger,
) {
	clientCtx := r.Context()

	rw := &responseWriter{ResponseWriter: w}
	if h.requestLoggingEnabled() {
		rw.EnableCapture()
	}
	responseLogged := false
	logStreamingResponse := func(outcome string) {
		if responseLogged || !h.requestLoggingEnabled() {
			return
		}
		responseLogged = true
		statusCode := rw.StatusCode()
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		h.logOutboundResponse(requestLogger, statusCode, rw.Header().Clone(), rw.CapturedBody(), true, outcome)
	}

	// Set SSE headers immediately
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rw.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Start heartbeat
	var finished int32
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if atomic.LoadInt32(&finished) == 1 {
					return
				}
				_, _ = fmt.Fprintf(rw, ":keepalive\n\n")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			case <-heartbeatDone:
				return
			case <-clientCtx.Done():
				return
			}
		}
	}()
	defer func() {
		atomic.StoreInt32(&finished, 1)
		close(heartbeatDone)
	}()

	streamStart := time.Now()

	for _, model := range modelChain {
		select {
		case <-clientCtx.Done():
			requestLogger.Info("client disconnected, stopping streaming fallbacks")
			logStreamingResponse("client_disconnected")
			return
		default:
		}

		requestLogger.Info("attempting streaming model", "model", model.ModelID, "provider", model.Provider)

		ctx, cancel := context.WithCancel(clientCtx)

		// Zen models use their own endpoint classification
		if client.IsZen(model) {
			endpointType := client.ClassifyEndpoint(model.ModelID)
			switch endpointType {
			case client.EndpointAnthropic:
				modelBody := replaceModelInRawBody(rawBody, model.ModelID)
				if err := h.handleAnthropicStreaming(ctx, rw, modelBody, model.ModelID, model); err != nil {
					cancel()
					if h.requestCanceledByDrain(clientCtx) {
						requestLogger.Warn("server draining during anthropic stream",
							"model", model.ModelID,
							"active_requests", h.lifecycle.ActiveRequests(),
						)
						logStreamingResponse("server_draining")
						return
					}
					if clientCtx.Err() == context.Canceled {
						requestLogger.Info("client disconnected during anthropic stream")
						logStreamingResponse("client_disconnected")
						return
					}
					h.logModelFailure(requestLogger, "anthropic streaming failed", model.ModelID, err)
					continue
				}
				cancel()
				latency := time.Since(streamStart)
				h.metrics.RecordSuccess(model.ModelID, latency)
				requestLogger.Info("streaming completed", "model", model.ModelID, "latency", latency)
				logStreamingResponse("completed")
				return

			case client.EndpointResponses:
				if err := h.handleResponsesStreaming(ctx, rw, anthropicReq, model, clientCtx); err != nil {
					cancel()
					if h.requestCanceledByDrain(clientCtx) {
						requestLogger.Warn("server draining during responses stream",
							"model", model.ModelID,
							"active_requests", h.lifecycle.ActiveRequests(),
						)
						logStreamingResponse("server_draining")
						return
					}
					if clientCtx.Err() == context.Canceled {
						requestLogger.Info("client disconnected during responses stream")
						logStreamingResponse("client_disconnected")
						return
					}
					h.logModelFailure(requestLogger, "responses streaming failed", model.ModelID, err)
					continue
				}
				cancel()
				latency := time.Since(streamStart)
				h.metrics.RecordSuccess(model.ModelID, latency)
				requestLogger.Info("streaming completed", "model", model.ModelID, "latency", latency)
				logStreamingResponse("completed")
				return

			case client.EndpointGemini:
				if err := h.handleGeminiStreaming(ctx, rw, anthropicReq, model, clientCtx); err != nil {
					cancel()
					if h.requestCanceledByDrain(clientCtx) {
						requestLogger.Warn("server draining during gemini stream",
							"model", model.ModelID,
							"active_requests", h.lifecycle.ActiveRequests(),
						)
						logStreamingResponse("server_draining")
						return
					}
					if clientCtx.Err() == context.Canceled {
						requestLogger.Info("client disconnected during gemini stream")
						logStreamingResponse("client_disconnected")
						return
					}
					h.logModelFailure(requestLogger, "gemini streaming failed", model.ModelID, err)
					continue
				}
				cancel()
				latency := time.Since(streamStart)
				h.metrics.RecordSuccess(model.ModelID, latency)
				requestLogger.Info("streaming completed", "model", model.ModelID, "latency", latency)
				logStreamingResponse("completed")
				return

			default:
				// Fall through to OpenAI-compatible handling
			}
		}

		// Anthropic endpoint on OpenCode Go (MiniMax + Qwen).
		if client.IsAnthropicModel(model.ModelID) {
			modelBody := replaceModelInRawBody(rawBody, model.ModelID)
			if err := h.handleAnthropicStreaming(ctx, rw, modelBody, model.ModelID, model); err != nil {
				cancel()
				if h.requestCanceledByDrain(clientCtx) {
					requestLogger.Warn("server draining during anthropic stream",
						"model", model.ModelID,
						"active_requests", h.lifecycle.ActiveRequests(),
					)
					logStreamingResponse("server_draining")
					return
				}
				if clientCtx.Err() == context.Canceled {
					requestLogger.Info("client disconnected during anthropic stream")
					logStreamingResponse("client_disconnected")
					return
				}
				h.logModelFailure(requestLogger, "anthropic streaming failed", model.ModelID, err)
				continue
			}
			cancel()
			latency := time.Since(streamStart)
			h.metrics.RecordSuccess(model.ModelID, latency)
			requestLogger.Info("streaming completed", "model", model.ModelID, "latency", latency)
			logStreamingResponse("completed")
			return
		}

		// OpenAI-compatible models (both Go and Zen)
		openaiReq, err := h.requestTransformer.TransformRequest(anthropicReq, model)
		if err != nil {
			cancel()
			h.logger.Warn("request transform failed", "model", model.ModelID, "error", err)
			continue
		}

		streamBody, err := h.client.GetStreamingBody(ctx, model.ModelID, openaiReq, model)
		if err != nil {
			cancel()
			if h.requestCanceledByDrain(clientCtx) {
				requestLogger.Warn("server draining during upstream request",
					"model", model.ModelID,
					"active_requests", h.lifecycle.ActiveRequests(),
				)
				logStreamingResponse("server_draining")
				return
			}
			if clientCtx.Err() == context.Canceled {
				requestLogger.Info("client disconnected during upstream request")
				logStreamingResponse("client_disconnected")
				return
			}
			h.logModelFailure(requestLogger, "streaming request failed", model.ModelID, err)
			continue
		}

		if err := h.streamHandler.ProxyStream(rw, streamBody, model.ModelID, clientCtx); err != nil {
			_ = streamBody.Close()
			cancel()
			if err == transformer.ErrClientDisconnected {
				if h.requestCanceledByDrain(clientCtx) {
					requestLogger.Warn("server draining during stream",
						"model", model.ModelID,
						"active_requests", h.lifecycle.ActiveRequests(),
					)
					logStreamingResponse("server_draining")
					return
				}
				requestLogger.Info("client disconnected during stream")
				logStreamingResponse("client_disconnected")
				return
			}
			if h.requestCanceledByDrain(clientCtx) {
				requestLogger.Warn("server draining during stream",
					"model", model.ModelID,
					"active_requests", h.lifecycle.ActiveRequests(),
				)
				logStreamingResponse("server_draining")
				return
			}
			if clientCtx.Err() == context.Canceled {
				requestLogger.Info("client disconnected during stream (context canceled)")
				logStreamingResponse("client_disconnected")
				return
			}
			h.logModelFailure(requestLogger, "stream proxy failed", model.ModelID, err)
			continue
		}

		_ = streamBody.Close()
		cancel()
		latency := time.Since(streamStart)
		h.metrics.RecordSuccess(model.ModelID, latency)
		requestLogger.Info("streaming completed", "model", model.ModelID, "latency", latency)
		logStreamingResponse("completed")
		return
	}

	h.metrics.RecordFailure()
	if !rw.wroteHeader {
		h.sendError(requestLogger, w, http.StatusBadGateway, "all streaming models failed", nil)
	} else {
		h.sendStreamError(requestLogger, rw, "all upstream models failed")
		logStreamingResponse("error")
	}
}

// handleResponsesStreaming handles streaming for OpenAI Responses endpoint.
func (h *MessagesHandler) handleResponsesStreaming(
	ctx context.Context,
	w http.ResponseWriter,
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
	clientCtx context.Context,
) error {
	req, err := h.requestTransformer.TransformToResponses(anthropicReq, model)
	if err != nil {
		return fmt.Errorf("responses transform failed: %w", err)
	}

	streamBody, err := h.client.GetResponsesStreamingBody(ctx, model.ModelID, req, model)
	if err != nil {
		return err
	}

	if err := h.streamHandler.ProxyResponsesStream(w, streamBody, model.ModelID, clientCtx); err != nil {
		_ = streamBody.Close()
		return err
	}

	_ = streamBody.Close()
	return nil
}

// handleGeminiStreaming handles streaming for Gemini endpoint.
func (h *MessagesHandler) handleGeminiStreaming(
	ctx context.Context,
	w http.ResponseWriter,
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
	clientCtx context.Context,
) error {
	req, err := h.requestTransformer.TransformToGemini(anthropicReq, model)
	if err != nil {
		return fmt.Errorf("gemini transform failed: %w", err)
	}

	streamBody, err := h.client.GetGeminiStreamingBody(ctx, model.ModelID, req, model)
	if err != nil {
		return err
	}

	if err := h.streamHandler.ProxyGeminiStream(w, streamBody, model.ModelID, clientCtx); err != nil {
		_ = streamBody.Close()
		return err
	}

	_ = streamBody.Close()
	return nil
}

// replaceModelInRawBody replaces the model field in raw JSON body with the actual model ID.
func replaceModelInRawBody(rawBody json.RawMessage, modelID string) json.RawMessage {
	bodyStr := string(rawBody)

	if idx := strings.Index(bodyStr, `"model":"`); idx != -1 {
		start := idx + len(`"model":"`)
		if end := strings.Index(bodyStr[start:], `"`); end != -1 {
			oldModel := bodyStr[start : start+end]
			newBody := bodyStr[:start] + modelID + bodyStr[start+end:]
			slog.Debug("replaced model in request body",
				"old_model", oldModel,
				"new_model", modelID,
				"success", true)
			return json.RawMessage(newBody)
		}
	}

	slog.Warn("could not find model field in request body, using original",
		"body_preview", bodyStr[:min(len(bodyStr), 200)])
	return rawBody
}

// handleAnthropicStreaming sends a raw Anthropic request to the Anthropic endpoint.
func (h *MessagesHandler) handleAnthropicStreaming(
	ctx context.Context,
	w http.ResponseWriter,
	rawBody json.RawMessage,
	modelID string,
	model config.ModelConfig,
) error {
	h.logger.Debug("sending anthropic streaming request",
		"model_id", modelID,
		"body_preview", string(rawBody)[:min(len(rawBody), 200)])

	rawBody = replaceModelInRawBody(rawBody, modelID)
	resp, err := h.client.SendAnthropicRequest(ctx, rawBody, true, model)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return transformer.ErrClientDisconnected
		}
		return fmt.Errorf("failed to copy response: %w", err)
	}

	return nil
}

// sendStreamError sends an error event in the SSE stream.
func (h *MessagesHandler) sendStreamError(logger *slog.Logger, w http.ResponseWriter, message string) {
	logger.Error("sending stream error", "message", message)

	errorEvent := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "api_error",
			"message": message,
		},
	}

	data, _ := json.Marshal(errorEvent)
	_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", string(data))

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// handleNonStreaming handles a non-streaming request with fallback.
func (h *MessagesHandler) handleNonStreaming(
	w http.ResponseWriter,
	r *http.Request,
	anthropicReq *types.MessageRequest,
	modelChain []config.ModelConfig,
	rawBody json.RawMessage,
	requestLogger *slog.Logger,
) {
	ctx := r.Context()
	startTime := time.Now()

	result, responseBody, err := h.fallbackHandler.ExecuteWithFallback(
		ctx,
		modelChain,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			// Zen models use their own endpoint classification
			if client.IsZen(model) {
				endpointType := client.ClassifyEndpoint(model.ModelID)
				switch endpointType {
				case client.EndpointAnthropic:
					return h.executeAnthropicRequest(ctx, rawBody, model)
				case client.EndpointResponses:
					return h.executeResponsesRequest(ctx, anthropicReq, model)
				case client.EndpointGemini:
					return h.executeGeminiRequest(ctx, anthropicReq, model)
				default:
					// Fall through to OpenAI-compatible handling
				}
			} else if client.IsAnthropicModel(model.ModelID) {
				// Go provider Anthropic-native models (MiniMax, Qwen)
				return h.executeAnthropicRequest(ctx, rawBody, model)
			}

			// OpenAI-compatible models (both Go and Zen)
			return h.executeOpenAIRequest(ctx, anthropicReq, model)
		},
	)

	if err != nil {
		h.metrics.RecordFailure()
		if h.requestCanceledByDrain(ctx) {
			w.Header().Set("Retry-After", "5")
			h.sendError(requestLogger, w, http.StatusServiceUnavailable, "server is restarting, please retry", err)
			return
		}
		h.sendError(requestLogger, w, http.StatusBadGateway, "all models failed", err)
		return
	}

	latency := time.Since(startTime)
	h.metrics.RecordSuccess(result.ModelID, latency)

	requestLogger.Info("request completed",
		"model", result.ModelID,
		"attempts", result.Attempted,
		"latency", latency,
	)
	h.logOutboundResponse(
		requestLogger,
		http.StatusOK,
		http.Header{"Content-Type": []string{"application/json"}},
		string(responseBody),
		false,
		"completed",
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseBody)
}

// executeAnthropicRequest executes a request to the Anthropic endpoint (for MiniMax models).
func (h *MessagesHandler) executeAnthropicRequest(
	ctx context.Context,
	rawBody json.RawMessage,
	model config.ModelConfig,
) ([]byte, error) {
	rawBody = replaceModelInRawBody(rawBody, model.ModelID)
	resp, err := h.client.SendAnthropicRequest(ctx, rawBody, false, model)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	h.logger.Debug("anthropic response", "body", string(body))

	return body, nil
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

// executeResponsesRequest executes a request to the OpenAI Responses endpoint.
func (h *MessagesHandler) executeResponsesRequest(
	ctx context.Context,
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
) ([]byte, error) {
	req, err := h.requestTransformer.TransformToResponses(anthropicReq, model)
	if err != nil {
		return nil, fmt.Errorf("responses transform failed: %w", err)
	}

	resp, err := h.client.ResponsesCompletionNonStreaming(ctx, model.ModelID, req, model)
	if err != nil {
		return nil, fmt.Errorf("responses completion failed: %w", err)
	}

	anthropicResp, err := h.responseTransformer.TransformResponsesResponse(resp, model.ModelID)
	if err != nil {
		return nil, fmt.Errorf("response transform failed: %w", err)
	}

	return json.Marshal(anthropicResp)
}

// executeGeminiRequest executes a request to the Gemini endpoint.
func (h *MessagesHandler) executeGeminiRequest(
	ctx context.Context,
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
) ([]byte, error) {
	req, err := h.requestTransformer.TransformToGemini(anthropicReq, model)
	if err != nil {
		return nil, fmt.Errorf("gemini transform failed: %w", err)
	}

	resp, err := h.client.GeminiCompletionNonStreaming(ctx, model.ModelID, req, model)
	if err != nil {
		return nil, fmt.Errorf("gemini completion failed: %w", err)
	}

	anthropicResp, err := h.responseTransformer.TransformGeminiResponse(resp, model.ModelID)
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

func (h *MessagesHandler) requestCanceledByDrain(ctx context.Context) bool {
	return ctx.Err() == context.Canceled && h.lifecycle != nil && h.lifecycle.IsDraining()
}

func (h *MessagesHandler) logModelFailure(logger *slog.Logger, message string, modelID string, err error) {
	fields := []any{"model", modelID, "error", err}
	fields = append(fields, client.ErrorAttrs(err)...)
	logger.Warn(message, fields...)
}

// sendError sends an error response in Anthropic format.
func (h *MessagesHandler) sendError(logger *slog.Logger, w http.ResponseWriter, statusCode int, message string, err error) {
	fields := []any{
		"status", statusCode,
		"message", message,
		"error", err,
	}
	fields = append(fields, client.ErrorAttrs(err)...)
	logger.Error("request error", fields...)

	if rw, ok := w.(*responseWriter); ok && rw.wroteHeader {
		return
	}

	errorResp := transformer.TransformErrorResponse(statusCode, message)
	body, marshalErr := json.Marshal(errorResp)
	if marshalErr != nil {
		body = []byte(`{"type":"error","error":{"type":"api_error","message":"failed to marshal error response"}}`)
	}
	body = append(body, '\n')
	h.logOutboundResponse(
		logger,
		statusCode,
		http.Header{"Content-Type": []string{"application/json"}},
		string(body),
		false,
		"error",
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}
