package handlers

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/routatic/proxy/internal/buildinfo"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/status"
	"github.com/routatic/proxy/internal/token"
	"github.com/routatic/proxy/pkg/types"
)

// HealthHandler handles health checks and token counting endpoints.
type HealthHandler struct {
	tokenCounter    *token.Counter
	fallbackHandler *router.FallbackHandler
	metrics         *metrics.Metrics
	statusStore     *status.Store
	// readiness, when set, can declare this instance unable to serve. Without
	// it the endpoint answered 200 unconditionally, so a release that started
	// with no usable credential was indistinguishable from a working one - and
	// the deploy gate only asks this endpoint.
	readiness func() (reason string, degraded bool)
	// directPath, when set, reports the direct Anthropic passthrough's current
	// verdict. It is informational: readiness already decides whether it is
	// degraded, and this is what lets an operator see the path being skipped
	// without reading the journal.
	directPath func() bool
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(tokenCounter *token.Counter, fallbackHandler *router.FallbackHandler, metrics *metrics.Metrics, statusStore *status.Store) *HealthHandler {
	return &HealthHandler{
		tokenCounter:    tokenCounter,
		fallbackHandler: fallbackHandler,
		metrics:         metrics,
		statusStore:     statusStore,
	}
}

// SetReadiness installs the check that lets /health fail. It must only report
// durable conditions: a transient one would roll back a healthy release.
func (h *HealthHandler) SetReadiness(fn func() (reason string, degraded bool)) {
	h.readiness = fn
}

// SetDirectPathStatus installs the direct passthrough's availability verdict.
func (h *HealthHandler) SetDirectPathStatus(fn func() bool) {
	h.directPath = fn
}

// HandleHealth handles GET /health.
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	// Get metrics snapshot
	snapshot := h.metrics.GetSnapshot()

	// Get circuit breaker states
	cbStates := map[string]string{}
	if h.fallbackHandler != nil {
		cbStates = h.fallbackHandler.GetCircuitStates()
	}

	status, code := "ok", http.StatusOK
	reason := ""
	if h.readiness != nil {
		if why, degraded := h.readiness(); degraded {
			status, code, reason = "degraded", http.StatusServiceUnavailable, why
		}
	}

	response := map[string]interface{}{
		"status":     status,
		"service":    "routatic-proxy",
		"version":    buildinfo.Version,
		"build_time": buildinfo.Date,
		"pid":        os.Getpid(),
		"binary":     buildinfo.BinaryPath(),
		"metrics": map[string]interface{}{
			"requests_received": snapshot.RequestsReceived,
			"requests_success":  snapshot.RequestsSuccess,
			"requests_failed":   snapshot.RequestsFailed,
			"requests_streamed": snapshot.RequestsStreamed,
			"upstream_calls":    snapshot.UpstreamCalls,
			"rate_limited":      snapshot.RateLimited,
			"p95_latency_ms":    snapshot.CalculateP95().Milliseconds(),
			"p99_latency_ms":    snapshot.CalculateP99().Milliseconds(),
		},
		"circuit_breakers": cbStates,
		"models":           snapshot.ModelCounts,
	}
	if h.directPath != nil {
		response["direct_path"] = map[string]interface{}{"unavailable": h.directPath()}
	}
	if reason != "" {
		response["reason"] = reason
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(response)
}

// HandleStatusline serves the current proxy status snapshot as JSON for
// consumption by the CLI status-line feature. It returns a status.Snapshot
// (with schema version, source label, and staleness flag) so the CLI can
// display real-time proxy health without polling the full /health endpoint.
func (h *HealthHandler) HandleStatusline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if h.statusStore == nil {
		_ = json.NewEncoder(w).Encode(status.Snapshot{SchemaVersion: 1, Source: "empty", Stale: true})
		return
	}
	_ = json.NewEncoder(w).Encode(h.statusStore.Snapshot())
}

// HandleCountTokens handles POST /v1/messages/count_tokens.
func (h *HealthHandler) HandleCountTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body types.MessageRequest

	// The only entry point that had neither a size cap nor admission. The
	// decoder allocates whatever the body declares, so one request from a local
	// process - or a cross-site page - could take the process down and every
	// stream with it.
	r.Body = http.MaxBytesReader(w, r.Body, maxAnthropicBodySize)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Count tokens.
	systemText, err := systemAndToolsTokenText(body.SystemText(), body.Tools)
	if err != nil {
		http.Error(w, "failed to process tools", http.StatusBadRequest)
		return
	}
	messages := tokenMessagesFromAnthropic(body.Messages)
	count := h.tokenCounter.CountMessages(systemText, messages)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]int{
		"input_tokens": count,
		"token_count":  count,
	})
}
