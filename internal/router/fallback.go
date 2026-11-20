// Package router selects models: scenario detection, fallback chains, circuit
// breakers, cost-based selection and the catalog lookups they share.
package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation — requests flow freely
	CircuitHalfOpen                     // Recovery probe — allowing limited test requests
	CircuitOpen                         // Failing fast — blocking all requests until timeout
)

// CircuitBreaker tracks failure rates and prevents calls to failing models.
type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	failureCount     int
	successCount     int
	lastFailureTime  time.Time
	threshold        int           // failures before opening circuit
	recoveryTimeout  time.Duration // how long to wait before half-open
	halfOpenMaxCalls int           // max test calls in half-open state
	halfOpenCalls    int
	generation       uint64
}

// NewCircuitBreaker creates a circuit breaker that opens after threshold
// consecutive failures and stays open for recoveryTimeout before allowing
// a probe. The defaults in NewFallbackHandler are 3 failures and 30s timeout,
// which balances quick recovery from transient issues with protection against
// sustained outages.
func NewCircuitBreaker(threshold int, recoveryTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            CircuitClosed,
		threshold:        threshold,
		recoveryTimeout:  recoveryTimeout,
		halfOpenMaxCalls: 3,
	}
}

// allowRequest returns the generation and whether to permit a request. In the
// closed state, all requests pass. In the open state, requests are blocked
// until recoveryTimeout elapses, at which point the circuit transitions to
// half-open and allows a limited number of concurrent probes.
func (cb *CircuitBreaker) allowRequest() (uint64, bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return cb.generation, true
	case CircuitOpen:
		// Check if recovery timeout has elapsed
		if time.Since(cb.lastFailureTime) > cb.recoveryTimeout {
			cb.state = CircuitHalfOpen
			cb.generation++
			cb.halfOpenCalls = 1
			return cb.generation, true
		}
		return 0, false
	case CircuitHalfOpen:
		if cb.halfOpenCalls < cb.halfOpenMaxCalls {
			cb.halfOpenCalls++
			return cb.generation, true
		}
		return 0, false
	}
	return 0, false
}

// recordSuccess requires mu and transitions toward a healthy state. In half-open
// mode, accumulating enough successes (halfOpenMaxCalls, default 3) closes
// the circuit. In closed mode, this resets the failure counter so transient
// failures don't accumulate over time.
func (cb *CircuitBreaker) recordSuccess() {
	switch cb.state {
	case CircuitHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.halfOpenMaxCalls {
			cb.state = CircuitClosed
			cb.generation++
			cb.failureCount = 0
			cb.successCount = 0
		}
	case CircuitClosed:
		cb.failureCount = 0
	}
}

// recordFailure requires mu and transitions toward an unhealthy state. In half-open
// mode, any failure immediately reopens the circuit. In closed mode, once the
// failure count reaches the threshold, the circuit opens and blocks subsequent
// requests until the recovery timeout elapses.
func (cb *CircuitBreaker) recordFailure() {
	cb.lastFailureTime = time.Now()
	cb.failureCount++

	switch cb.state {
	case CircuitHalfOpen:
		cb.state = CircuitOpen
		cb.generation++
		cb.successCount = 0
	case CircuitClosed:
		if cb.failureCount >= cb.threshold {
			cb.state = CircuitOpen
			cb.generation++
		}
	}
}

// State returns the current circuit breaker state (CircuitClosed, CircuitHalfOpen,
// or CircuitOpen). Use this to inspect whether a model is being skipped due to
// recent failures, or to expose circuit state via metrics/health endpoints.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// FallbackResult contains the result of a fallback attempt.
type FallbackResult struct {
	ModelID           string
	Provider          string
	Attempted         int
	CandidatePosition int // one-based position in the execution chain; zero when no result was served
}

// FallbackHandler manages model fallback with circuit breaker protection.
type FallbackHandler struct {
	logger          *slog.Logger
	circuitBreakers map[string]*CircuitBreaker
	cbThreshold     int
	cbTimeout       time.Duration
	mu              sync.Mutex
	atomicCfg       *config.AtomicConfig // Optional: for checking provider key counts
	keyAvailability KeyAvailability      // Optional: live key health, preferred over the count
}

// NewFallbackHandler creates a handler that tries models in sequence until one
// succeeds, with per-model circuit breakers to skip failing models. Use this
// when you need resilient upstream calls with automatic backoff — the handler
// tracks failures per model and avoids hammering an already-failing endpoint.
// Default threshold is 3 failures; default timeout is 30 seconds.
func NewFallbackHandler(logger *slog.Logger, cbThreshold int, cbTimeout time.Duration) *FallbackHandler {
	if logger == nil {
		logger = slog.Default()
	}
	if cbThreshold <= 0 {
		cbThreshold = 3
	}
	if cbTimeout <= 0 {
		cbTimeout = 30 * time.Second
	}

	return &FallbackHandler{
		logger:          logger,
		circuitBreakers: make(map[string]*CircuitBreaker),
		cbThreshold:     cbThreshold,
		cbTimeout:       cbTimeout,
	}
}

// KeyAvailability reports whether a provider has any key able to take a request
// right now. It is satisfied by the provider layer's key scheduler.
//
// The fallback handler needs this to decide whether a 401 says "this credential
// is bad, try the platform's next model" or "this platform has nothing left to
// try". Counting keys is not enough: a platform with three keys can have all
// three resting, and continuing down its model chain would then spend one
// rejected request per model.
type KeyAvailability interface {
	// HasUsableKey reports whether the named provider has at least one key that
	// is not resting. A provider with no keys configured reports false.
	HasUsableKey(provider string) bool
}

// SetAtomicConfig sets the atomic config for the fallback handler.
// This is used to check provider key counts for auth error handling.
func (h *FallbackHandler) SetAtomicConfig(cfg *config.AtomicConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.atomicCfg = cfg
}

// SetKeyAvailability installs the key-health source used to decide whether an
// auth failure should skip the rest of a provider's models.
func (h *FallbackHandler) SetKeyAvailability(source KeyAvailability) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.keyAvailability = source
}

// providerHasUsableKey reports whether the provider can still take a request.
//
// Without a key-health source it falls back to the older rule - more than one
// configured key means "another key may work" - so the handler behaves sensibly
// when it is constructed without one.
func (h *FallbackHandler) providerHasUsableKey(provider string) bool {
	h.mu.Lock()
	cfg, source := h.atomicCfg, h.keyAvailability
	h.mu.Unlock()
	if source != nil {
		return source.HasUsableKey(provider)
	}
	return client.ProviderKeyCount(cfg, provider) > 1
}

// getCircuitBreaker returns or creates a circuit breaker for a model.
func (h *FallbackHandler) getCircuitBreaker(modelID string) *CircuitBreaker {
	h.mu.Lock()
	defer h.mu.Unlock()

	cb, exists := h.circuitBreakers[modelID]
	if !exists {
		cb = NewCircuitBreaker(h.cbThreshold, h.cbTimeout)
		h.circuitBreakers[modelID] = cb
	}
	return cb
}

// AllowAttempt returns a once-only completion function, or nil when blocked.
// Every admitted attempt must complete, including neutral/canceled outcomes.
func (h *FallbackHandler) AllowAttempt(model config.ModelConfig) func(context.Context, error) bool {
	cb := h.getCircuitBreaker(config.ModelKey(model))
	generation, allowed := cb.allowRequest()
	if !allowed {
		return nil
	}
	var once sync.Once
	var blockProvider bool
	return func(ctx context.Context, err error) bool {
		once.Do(func() {
			blockProvider = h.recordAttempt(ctx, model, err, cb, generation)
		})
		return blockProvider
	}
}

// recordAttempt updates shared health and reports whether the rest of this
// provider should be skipped for this request. ctx is the client's context, not
// the per-attempt timeout: disconnects must not penalize a healthy upstream.
func (h *FallbackHandler) recordAttempt(ctx context.Context, model config.ModelConfig, err error, cb *CircuitBreaker, generation uint64) bool {
	var source *core.ResponseError
	if errors.As(err, &source) {
		err = source.Err // Delivery errors joined for history must not vote on source health.
	}
	blockProvider := IsUsageLimitError(err) || errors.Is(err, core.ErrAllKeysCooling)
	if IsAuthError(err) {
		// An auth failure says the credential is bad. Whether that should skip
		// the platform's remaining models depends on whether another key can
		// take over: if one can, the next model is worth trying; if none can,
		// every further model would be rejected the same way.
		blockProvider = !h.providerHasUsableKey(client.Provider(model))
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	// A late completion must not release or vote in a newer recovery round.
	current := cb.generation == generation
	if current && cb.state == CircuitHalfOpen {
		cb.halfOpenCalls--
	}
	if ctx.Err() != nil {
		return false
	}
	if err == nil {
		if current {
			cb.recordSuccess()
		}
		return false
	}
	if blockProvider || IsAuthError(err) {
		return blockProvider
	}
	if current && IsRetryableError(err) {
		cb.recordFailure()
	}
	return false
}

// ExecuteWithFallback tries models in sequence until one succeeds.
// Respects circuit breaker state to skip models that are failing repeatedly.
// The executor receives the one-based execution count; skipped candidates do
// not execute. The separate one-based candidate position identifies fallbacks.
// This is not an HTTP count: key selection can fail before sending.
func (h *FallbackHandler) ExecuteWithFallback(
	ctx context.Context,
	models []config.ModelConfig,
	executor func(ctx context.Context, model config.ModelConfig, attempt, candidatePosition int) ([]byte, error),
) (*FallbackResult, []byte, error) {
	totalModels := len(models)
	if totalModels == 0 {
		return &FallbackResult{}, nil, errors.New("no models available for fallback")
	}
	blockedProviders := make(map[string]bool)
	var usageLimitErr error
	var usageLimitModel config.ModelConfig
	var usageLimitProvider string
	var authErr error
	var authModel config.ModelConfig
	var authProvider string
	// lastErr keeps the most recent attempt's error so the caller can report
	// why the request failed instead of only that it did. The proxy forwards
	// upstream refusals to the client, so discarding this turned every
	// deterministic failure into an opaque 502.
	var lastErr error
	attempted := 0
	// lastModel is the model the executor actually ran most recently. An
	// exhausted chain used to report models[0], which names a candidate that
	// may have been skipped by an open circuit breaker - the failure row and
	// the model counter then blamed a model that never saw the request.
	var lastModel config.ModelConfig
	lastProvider := ""

	for i, model := range models {
		if err := ctx.Err(); err != nil {
			h.logger.Info("request context canceled, stopping fallback attempts",
				"error", err,
			)
			return nil, nil, err
		}

		provider := client.Provider(model)
		if blockedProviders[provider] {
			h.logger.Info("provider unavailable for this request, skipping model", "provider", provider, "model", model.ModelID)
			continue
		}

		// Skip models with open circuit breakers
		complete := h.AllowAttempt(model)
		if complete == nil {
			h.logger.Info("circuit breaker open, skipping model",
				"model", model.ModelID,
				"candidate", i+1,
				"total", totalModels,
			)
			continue
		}

		attempted++
		lastModel, lastProvider = model, provider
		h.logger.Info("attempting model",
			"model", model.ModelID,
			"attempt", attempted,
			"candidate", i+1,
			"total", totalModels,
		)

		body, err := executor(ctx, model, attempted, i+1)
		blockProvider := complete(ctx, err)
		if err == nil {
			h.logger.Info("model succeeded",
				"model", model.ModelID,
				"attempt", attempted,
			)
			return &FallbackResult{
				ModelID:           model.ModelID,
				Provider:          provider,
				Attempted:         attempted,
				CandidatePosition: i + 1,
			}, body, nil
		}

		// A complete native response (including a failed one) is the result,
		// not a transport failure; late cancellation must not erase its accounting.
		var final interface{ FinalResponse() []byte }
		if errors.As(err, &final) && len(final.FinalResponse()) > 0 {
			return &FallbackResult{ModelID: model.ModelID, Provider: provider, Attempted: attempted, CandidatePosition: i + 1}, final.FinalResponse(), err
		}
		if errCtx := ctx.Err(); errCtx != nil {
			h.logger.Info("request context canceled after model attempt, stopping fallback",
				"model", model.ModelID,
				"error", errCtx,
			)
			return nil, nil, errCtx
		}

		// The request itself is unacceptable. Every model in the chain would
		// reject it the same way, so trying them spends quota on an answer that
		// is already known - and hides the real cause behind several attempts.
		if IsNonRetryableRequestError(err) {
			h.logger.Warn("request rejected as invalid, not falling back",
				"model", model.ModelID,
				"error", err,
			)
			return &FallbackResult{
				ModelID:           model.ModelID,
				Provider:          provider,
				Attempted:         attempted,
				CandidatePosition: i + 1,
			}, nil, err
		}

		// A provider-wide usage limit makes its remaining models pointless.
		// Skip them, but continue if the chain includes another provider.
		lastErr = err
		if blockProvider {
			blockedProviders[provider] = true
		}
		if IsUsageLimitError(err) {
			usageLimitErr, usageLimitModel, usageLimitProvider = err, model, provider
			h.logger.Warn("provider usage limit reached, trying another provider",
				"provider", provider,
				"model", model.ModelID,
				"error", err,
			)
			continue
		}

		// Auth errors (401/403) indicate invalid credentials.
		// If the provider has a single API key, block it so its remaining
		// models are skipped. If it has multiple keys, don't block the
		// round-robin — the next attempt will use a different key.
		if IsAuthError(err) {
			if blockProvider {
				h.logger.Warn("authentication error, blocking provider",
					"provider", provider,
					"model", model.ModelID,
					"error", err,
				)
				authErr, authModel, authProvider = err, model, provider
				continue
			}
			h.logger.Warn("authentication error, but provider has multiple keys, trying next",
				"provider", provider,
				"model", model.ModelID,
				"error", err,
			)
		}

		if IsRetryableError(err) {
			h.logger.Warn("model failed, trying fallback",
				"model", model.ModelID,
				"error", err,
				"remaining", totalModels-i-1,
				"circuit_state", h.getCircuitBreaker(config.ModelKey(model)).State(),
			)
		} else {
			h.logger.Warn("non-retryable error (skipping circuit breaker), trying fallback",
				"model", model.ModelID,
				"error", err,
				"remaining", totalModels-i-1,
			)
		}
	}

	// A retained auth or usage-limit error names the attempt that produced it,
	// not the last one: a later model on another platform may have failed for
	// an unrelated reason, and the client is shown the retained error.
	if authErr != nil {
		return &FallbackResult{
			ModelID:   authModel.ModelID,
			Provider:  authProvider,
			Attempted: attempted,
		}, nil, authErr
	}

	if usageLimitErr != nil {
		return &FallbackResult{
			ModelID:   usageLimitModel.ModelID,
			Provider:  usageLimitProvider,
			Attempted: attempted,
		}, nil, usageLimitErr
	}

	// Every candidate can be skipped before it is ever sent - all circuit
	// breakers open, or the provider already blocked on a usage limit. There is
	// then no upstream error to report, and %w would render a nil operand.
	if lastErr == nil {
		return &FallbackResult{
			ModelID:   lastModel.ModelID,
			Provider:  lastProvider,
			Attempted: attempted,
		}, nil, fmt.Errorf("%w (%d execution attempts, %d candidates): no candidate was attempted", ErrAllModelsFailed, attempted, totalModels)
	}

	return &FallbackResult{
		ModelID:   lastModel.ModelID,
		Provider:  lastProvider,
		Attempted: attempted,
	}, nil, fmt.Errorf("%w (%d execution attempts, %d candidates): %w", ErrAllModelsFailed, attempted, totalModels, lastErr)
}

// ErrAllModelsFailed marks an exhausted chain. Callers classify it with
// errors.Is rather than matching the message text, so the wording can change
// without breaking the mapping.
var ErrAllModelsFailed = errors.New("all models failed")

// ErrWithinSiteNoTarget marks a chain that narrowed to the active platform and
// came up empty. It is the operator's configuration rather than an upstream
// failure, so it is reported as such instead of as a gateway error.
var ErrWithinSiteNoTarget = errors.New("active site has no routing target")

// IsRetryableError determines if an error is worth retrying with a fallback.
func IsRetryableError(err error) bool {
	if err == nil || errors.Is(err, core.ErrAllKeysCooling) {
		return false
	}

	// APIError from the client carries the HTTP status code — use it directly
	// instead of string matching, so error format changes upstream can't
	// silently break the classification.
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		// 4xx client errors are not retryable — the request format itself is
		// invalid for that model, and retrying won't fix it. This includes 429
		// (rate limit) so the circuit breaker doesn't open for rate limits.
		return apiErr.StatusCode >= 500
	}

	// Wrapped deadlines and network timeouts need not contain "timeout".
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// A stream that stalled or ended early failed after its headers arrived. The
	// HTTP status looked fine, so nothing else in this classifier sees it - and
	// without a vote the same dead upstream is picked for every later request.
	if errors.Is(err, core.ErrUpstreamStreamFailed) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Retain legacy classification for untyped upstream errors.
	errStr := err.Error()

	retryable := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"rate limit",
		"503",
		"502",
		"500",
	}

	for _, sub := range retryable {
		if strings.Contains(errStr, sub) {
			return true
		}
	}
	return false
}

// IsNonRetryableRequestError reports whether an error says the request itself is
// wrong, rather than the model failing to serve it.
//
// The distinction is the point of a fallback chain. A 5xx, a timeout or a 429
// describes this attempt - another platform may well do better. A 400, a 413 or
// a context-length rejection describes the request, and every model in the chain
// will refuse it the same way. Trying them in turn spends the operator's quota
// on an answer that is already known, and buries the real cause under several
// attempts.
//
// The sentence this enforces: a malformed request is the client's answer, and
// returning it immediately is both cheaper and more truthful.
func IsNonRetryableRequestError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusBadRequest, // 400 malformed body, bad schema
			http.StatusRequestEntityTooLarge, // 413 body too large
			http.StatusUnprocessableEntity,   // 422 semantically invalid
			http.StatusUnsupportedMediaType:  // 415
			return true
		}
		// Anthropic and OpenAI both report a context overrun as 400, and so do
		// most OpenAI-compatible gateways. Anything else in the 4xx family is
		// about the credential or the model's availability and stays retryable.
		return false
	}
	// A source response parse failure is not malformed client input.
	var responseErr *core.ResponseError
	if errors.As(err, &responseErr) {
		return false
	}
	// A request this proxy rejected on its own terms - an effort outside the
	// ladder, a malformed output_config - will be rejected the same way by
	// every candidate. This must stay narrower than RequestConversionError:
	// that type also covers model-specific conversion failures (cache
	// breakpoints, thinking history), which another target may accept.
	if errors.Is(err, core.ErrInvalidRequest) {
		return true
	}
	// An unclassified local body parse failure still rejects the request.
	var syntaxErr *json.SyntaxError
	return errors.As(err, &syntaxErr)
}

// IsUsageLimitError returns true if the error is a GoUsageLimitError.
// Usage limit errors should be passed directly to the client instead of
// triggering a fallback, as fallback attempts will also encounter the
// same usage limit within a short period.
func IsUsageLimitError(err error) bool {
	if err == nil {
		return false
	}

	// Check for GoUsageLimitError in the error message
	// The error body contains: {"type":"error","error":{"type":"GoUsageLimitError",...}}
	errStr := err.Error()
	return strings.Contains(errStr, "GoUsageLimitError")
}

// IsAuthError returns true if the error is an authentication error (401 or 403).
// Auth errors are non-retryable and indicate invalid or expired credentials.
// Since all models from the same provider share the same API key, fallback
// attempts will fail identically, so we short-circuit the fallback chain.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 401 || apiErr.StatusCode == 403
	}
	return false
}

// GetCircuitStates returns the state of all circuit breakers.
func (h *FallbackHandler) GetCircuitStates() map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()

	states := make(map[string]string)
	for modelID, cb := range h.circuitBreakers {
		state := cb.State()
		switch state {
		case CircuitClosed:
			states[modelID] = "closed"
		case CircuitHalfOpen:
			states[modelID] = "half_open"
		case CircuitOpen:
			states[modelID] = "open"
		default:
			states[modelID] = "unknown"
		}
	}
	return states
}
