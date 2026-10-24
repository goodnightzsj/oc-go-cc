// Package gui provides the embedded HTTP server that serves the GUI dashboard
// and exposes /api/* endpoints for metrics, history, and configuration.
package gui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/daemon"
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/quota"
	"github.com/routatic/proxy/internal/site"
	"github.com/routatic/proxy/internal/storage"
)

//go:embed assets/*
var assets embed.FS

// Config is the GUI-level configuration that the user can toggle at runtime.
type Config struct {
	Autostart bool `json:"autostart"`
	Notify    bool `json:"notify"`
}

// Server is the embedded HTTP server that backs the webview UI.
type Server struct {
	met               *metrics.Metrics
	atomicCfg         *config.AtomicConfig
	cfg               Config
	cfgMu             sync.RWMutex
	proxyConfigMu     sync.Mutex
	proxyRunning      atomic.Bool
	connectedExisting atomic.Bool
	proxyPort         int
	guiPort           atomic.Int32
	startProxy        func() error
	stopProxy         func() error
	setAutostart      func(bool, int) error
	catalogDir        string
	catalogSourceURL  string
	// The catalog file is ~6.5MB and parsing it costs about 200ms, while this
	// endpoint is hit every time a picker opens. The model detail changes only
	// when the catalog is re-synced, so it is parsed once and reused until the
	// sync handler clears it.
	catalogModelsMu    sync.Mutex
	catalogModelsIndex *catalog.IndexedCatalog
	srv                *http.Server
	logger             *slog.Logger
	limitsCancel       context.CancelFunc
	limitsDone         chan struct{}
	channels           channelCatalog

	// One cached quota response per platform; each response carries its TTL.
	quotaMu    sync.Mutex
	quotaCache map[string]quotaCacheEntry
	// Coalesce concurrent explicit, billable AWS lookups; the fetch override is
	// used by isolated HTTP/browser tests, never by runtime configuration.
	bedrockBillingMu    sync.Mutex
	fetchBedrockBilling func(context.Context, config.AWSBillingConfig) (*quota.BedrockBilling, error)

	// Per-model allowance table synced daily from the Go docs; see
	// limitsRefreshTTL. modelLimitsURL overrides the docs URLs (tests).
	limitsMu       sync.Mutex
	modelLimits    *quota.ModelLimits
	modelLimitsURL []string

	// circuitStates reports the live fallback circuit breakers, keyed by model.
	// It is injected rather than computed here so the handler that owns that
	// state stays its only owner; nil means the dashboard omits breaker state
	// instead of inventing a healthy default.
	circuitStates func() map[string]string

	storage *storage.Database
}

// Options configures the GUI server.
type Options struct {
	Metrics          *metrics.Metrics
	AtomicConfig     *config.AtomicConfig
	ProxyPort        int
	StartProxy       func() error
	StopProxy        func() error
	CatalogDir       string
	CatalogSourceURL string
	Logger           *slog.Logger
	Storage          *storage.Database
	// CircuitStates returns the live circuit-breaker state per model. Omit it
	// when no fallback handler is wired; the dashboard then reports breaker
	// state as unknown rather than as closed.
	CircuitStates func() map[string]string
}

// New creates a new GUI server.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Server{
		met:              opts.Metrics,
		atomicCfg:        opts.AtomicConfig,
		proxyPort:        opts.ProxyPort,
		startProxy:       opts.StartProxy,
		stopProxy:        opts.StopProxy,
		catalogDir:       opts.CatalogDir,
		catalogSourceURL: opts.CatalogSourceURL,
		logger:           opts.Logger,
		circuitStates:    opts.CircuitStates,

		storage: opts.Storage,
	}
	// Check initial autostart state.
	s.cfg.Autostart = isAutostartEnabled()
	return s
}

// isAutostartEnabled checks whether autostart is currently enabled.
// On macOS it checks ~/Library/LaunchAgents/{LaunchAgent}.plist.
// On Linux it checks ~/.config/autostart/{LaunchAgent}.desktop.
func isAutostartEnabled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	if runtime.GOOS == "darwin" {
		plist := filepath.Join(home, "Library", "LaunchAgents", daemon.LaunchAgent+".plist")
		_, err = os.Stat(plist)
		return err == nil
	}

	if runtime.GOOS == "linux" {
		desktop := filepath.Join(home, ".config", "autostart", daemon.LaunchAgent+".desktop")
		_, err = os.Stat(desktop)
		return err == nil
	}

	return false
}

// SetProxyRunning updates the running state (called by the proxy lifecycle).
func (s *Server) SetProxyRunning(running bool) {
	s.proxyRunning.Store(running)
}

// SetConnectedToExisting updates whether the GUI is monitoring an external proxy
// rather than controlling a locally-started one.
func (s *Server) SetConnectedToExisting(connected bool) {
	s.connectedExisting.Store(connected)
}

// getProxyPort returns the current proxy port from config if available.
func (s *Server) getProxyPort() int {
	if s.atomicCfg != nil {
		return s.atomicCfg.Get().Port
	}
	return s.proxyPort
}

// Start starts the embedded HTTP server on port 3445 (or the next free port in
// the following ten) and returns the URL that the webview should load. A stale
// instance already holding the port is left alone; this one simply moves up.
func (s *Server) Start(ctx context.Context) (string, error) {
	s.guiPort.Store(3445)
	// Allow GUI port override via environment variable.
	if envPort := os.Getenv("ROUTATIC_PROXY_GUI_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 && p < 65536 {
			s.guiPort.Store(int32(p))
		}
	}

	mux := http.NewServeMux()

	// Content-hashed static assets. index.html is served from a rendered copy
	// whose <script>/<link> URLs carry a per-build content hash (e.g.
	// app.<sha12>.js), and those hashed URLs are served with an immutable
	// long cache header. A fresh deploy changes the content, so the hashed
	// URL changes and caches (incl. Cloudflare edge) naturally invalidate —
	// this is the fix for stale front-ends behind a CDN.
	hashed, err := buildHashedAssets()
	if err != nil {
		return "", err
	}
	mux.HandleFunc("/", hashed.serveIndex)
	for name, data := range hashed.files {
		mux.HandleFunc("/"+name, hashed.serveHashed(name, data))
	}

	// API endpoints.
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/api/history", s.handleHistory)
	mux.HandleFunc("/api/history/summary", s.handleHistorySummary)
	mux.HandleFunc("/api/history/channels", s.handleHistoryChannels)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/proxy/config", s.handleProxyConfig)
	mux.HandleFunc("/api/cline-pass/channels", s.handleChannelCatalog)
	mux.HandleFunc("/api/proxy/start", s.handleProxyStart)
	mux.HandleFunc("/api/proxy/stop", s.handleProxyStop)
	mux.HandleFunc("/api/catalog/lock", s.handleCatalogLock)
	mux.HandleFunc("/api/catalog/sync", s.handleCatalogSync)
	mux.HandleFunc("/api/catalog/models", s.handleCatalogModels)
	mux.HandleFunc("/api/prices", s.handlePrices)
	mux.HandleFunc("/api/quota", s.handleQuota)
	mux.HandleFunc("/api/sites", s.handleSites)
	mux.HandleFunc("/api/test/send", s.handleTestSend)

	// New endpoints for advanced GUI features

	mux.HandleFunc("/api/config/export", s.handleConfigExport)
	mux.HandleFunc("/api/config/import", s.handleConfigImport)
	mux.HandleFunc("/api/perf/models", s.handlePerformance)
	mux.HandleFunc("/api/perf/aggregate", s.handlePerformanceAggregate)

	// Analytics (only when SQLite storage is available)
	if s.storage != nil {
		ah := NewAnalyticsHandler(s.storage)
		mux.HandleFunc("/api/analytics/summary", ah.Summary)
		mux.HandleFunc("/api/analytics/tokens/trend", ah.TokenTrend)
	}

	startPort := int(s.guiPort.Load())
	var ln net.Listener
	for p := startPort; p < startPort+10; p++ {
		var err error
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			s.guiPort.Store(int32(p))
			break
		}
		if p == startPort {
			fmt.Printf("Port %d in use, trying port %d for GUI\n", p, p+1)
		}
	}
	if ln == nil {
		return "", fmt.Errorf("gui server listen: no available port in range %d-%d", startPort, startPort+9)
	}

	// Wrap with security headers middleware.
	s.srv = &http.Server{Handler: securityHeadersMiddleware(mux)}
	limitsCtx, cancel := context.WithCancel(ctx)
	s.limitsCancel = cancel
	s.limitsDone = make(chan struct{})
	go func() {
		defer close(s.limitsDone)
		var workers sync.WaitGroup
		workers.Add(1)
		go func() {
			defer workers.Done()
			s.channelLoop(limitsCtx)
		}()
		s.limitsLoop(limitsCtx)
		workers.Wait()
	}()
	go func() {
		if srvErr := s.srv.Serve(ln); srvErr != nil && srvErr != http.ErrServerClosed {
			s.logger.Error("gui server error", "err", srvErr)
		}
	}()

	url := "http://" + ln.Addr().String() + "/"
	s.logger.Info("gui server started", "url", url)
	return url, nil
}

// shortHash returns the first 12 hex chars of the SHA-256 of data.
func shortHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

// hashedAssets renders the embedded index.html with content-hashed asset URLs
// and exposes each hashed file for serving. Using a new hashed filename on
// every content change defeats stale edge/CDN caches (e.g. Cloudflare caching
// app.js for 7 days) without manual purges.
type hashedAssets struct {
	index []byte // rendered index.html referencing hashed URLs
	files map[string][]byte
}

func buildHashedAssets() (*hashedAssets, error) {
	h := &hashedAssets{files: map[string][]byte{}}

	read := func(name string) ([]byte, error) {
		b, err := fs.ReadFile(assets, "assets/"+name)
		if err != nil {
			return nil, fmt.Errorf("read embedded %s: %w", name, err)
		}
		return b, nil
	}

	indexHTML, err := read("index.html")
	if err != nil {
		return nil, err
	}
	appJS, err := read("app.js")
	if err != nil {
		return nil, err
	}
	styleCSS, err := read("style.css")
	if err != nil {
		return nil, err
	}
	twCSS, err := read("compiled-tailwind.css")
	if err != nil {
		return nil, err
	}
	faviconSVG, err := read("favicon.svg")
	if err != nil {
		return nil, err
	}

	// content-hashed filenames
	appName := "app." + shortHash(appJS) + ".js"
	styleName := "style." + shortHash(styleCSS) + ".css"
	twName := "compiled-tailwind." + shortHash(twCSS) + ".css"

	h.files[appName] = appJS
	h.files[styleName] = styleCSS
	h.files[twName] = twCSS
	// Keep plain names working too (backward-compat for direct fetches).
	h.files["app.js"] = appJS
	h.files["style.css"] = styleCSS
	h.files["compiled-tailwind.css"] = twCSS
	// The favicon keeps a stable URL on purpose: browsers cache tab icons
	// aggressively under their own rules and often refetch /favicon.svg
	// directly, so a content-hashed name would just be missed.
	h.files["favicon.svg"] = faviconSVG

	// Rewrite references in index.html to the hashed names.
	html := string(indexHTML)
	html = strings.Replace(html, `href="compiled-tailwind.css"`, `href="`+twName+`"`, 1)
	html = strings.Replace(html, `href="style.css"`, `href="`+styleName+`"`, 1)
	html = strings.Replace(html, `src="app.js"`, `src="`+appName+`"`, 1)
	// Expose the app.js content-hash in the served HTML so the user can see at
	// a glance which UI build a browser actually loaded (diagnoses stale-cache).
	build := shortHash(appJS)
	uiBadge := `<div id="ui-build" class="ui-build" data-build="` + build + `" title="UI build ` + build + `">UI ` + build + `</div>`
	html = strings.Replace(html, `<div id="ui-build" class="ui-build" title=""></div>`, uiBadge, 1)
	h.index = []byte(html)

	return h, nil
}

// serveIndex serves the rendered (hashed) index.html.
func (h *hashedAssets) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(h.index)
}

// serveHashed serves a static asset. Hashed (immutable) files get a long cache
// lifetime; plain-name aliases stay no-cache so stale CDN copies expire.
func (h *hashedAssets) serveHashed(name string, data []byte) http.HandlerFunc {
	hashed := isHashedName(name)
	return func(w http.ResponseWriter, r *http.Request) {
		ct := "application/javascript; charset=utf-8"
		switch {
		case strings.HasSuffix(name, ".css"):
			ct = "text/css; charset=utf-8"
		case strings.HasSuffix(name, ".svg"):
			ct = "image/svg+xml"
		}
		w.Header().Set("Content-Type", ct)
		if hashed {
			// Content-addressed: safe to cache forever; invalidation is the
			// new filename, which the hashed index.html points at.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		_, _ = w.Write(data)
	}
}

// isHashedName reports whether name carries a content-hash prefix (x.<12hex>.<ext>).
func isHashedName(name string) bool {
	base := name[:strings.LastIndex(name, ".")]
	i := strings.LastIndex(base, ".")
	if i < 0 {
		return false
	}
	tag := base[i+1:]
	if len(tag) != 12 {
		return false
	}
	_, err := hex.DecodeString(tag)
	return err == nil
}

// Shutdown gracefully stops the GUI server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	s.limitsCancel()
	err := s.srv.Shutdown(ctx)
	select {
	case <-s.limitsDone:
		return err
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
}

// ── API handlers ──────────────────────────────────────────────────────────────

type metricsResponse struct {
	ProxyRunning      bool             `json:"proxy_running"`
	ConnectedExisting bool             `json:"connected_to_existing"`
	Port              int              `json:"port"`
	RequestsReceived  int64            `json:"requests_received"`
	RequestsStreamed  int64            `json:"requests_streamed"`
	RequestsSuccess   int64            `json:"requests_success"`
	RequestsFailed    int64            `json:"requests_failed"`
	ModelCounts       map[string]int64 `json:"model_counts"`
	// CircuitBreakers maps a model key to closed/half_open/open. Absent when no
	// fallback handler is wired, which the dashboard shows as unknown.
	CircuitBreakers map[string]string `json:"circuit_breakers,omitempty"`
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	var snap metrics.Snapshot
	if s.met != nil {
		snap = s.met.GetSnapshot()
	}
	// The metrics counters live in process memory and reset on restart, which
	// left the overview page reading all zeros even with thousands of requests
	// on record. Fall back to the persisted roll-up in that case.
	if snap.RequestsReceived == 0 && s.storage != nil {
		if totals, err := storage.NewRequests(s.storage).Totals(); err == nil && totals.Received > 0 {
			snap.RequestsReceived = totals.Received
			snap.RequestsStreamed = totals.Streamed
			snap.RequestsSuccess = totals.Success
			snap.RequestsFailed = totals.Failed
			snap.ModelCounts = totals.ModelCounts
		}
	}
	resp := metricsResponse{
		ProxyRunning:      s.proxyRunning.Load(),
		ConnectedExisting: s.connectedExisting.Load(),
		Port:              s.getProxyPort(),
		RequestsReceived:  snap.RequestsReceived,
		RequestsStreamed:  snap.RequestsStreamed,
		RequestsSuccess:   snap.RequestsSuccess,
		RequestsFailed:    snap.RequestsFailed,
		ModelCounts:       snap.ModelCounts,
	}
	if s.circuitStates != nil {
		resp.CircuitBreakers = s.circuitStates()
	}
	writeJSON(w, resp)
}

type historyEntry struct {
	ID    string `json:"id"`
	Model string `json:"model"`
	// RequestedModel is the model the client named. Omitted when it matched the
	// served model, so the field's presence is itself the signal that this
	// request was routed somewhere other than what it asked for.
	RequestedModel string `json:"requested_model,omitempty"`
	Provider       string `json:"provider"`
	// UpstreamChannel is the channel the upstream gateway reported serving the
	// request on. Only ClinePass publishes one, so it is empty everywhere else -
	// which is why the UI shows the badge for that platform only rather than
	// rendering an empty marker on every row.
	UpstreamChannel string `json:"upstream_channel,omitempty"`
	// UpstreamModel is the model the gateway said it ran, when that is not the
	// id that was requested. A gateway can answer one id by running a virtual
	// model behind its own fallback chain, so the model column stops describing
	// what produced the response; this is the field that does.
	UpstreamModel string `json:"upstream_model,omitempty"`
	// FirstByteMs is how long the upstream took to send its first byte. Omitted
	// when it was not measured, which is a different answer from zero.
	FirstByteMs               int64                     `json:"first_byte_ms,omitempty"`
	FirstEventMs              *int64                    `json:"first_event_ms,omitempty"`
	FirstContentMs            *int64                    `json:"first_content_ms,omitempty"`
	MessageEndMs              *int64                    `json:"message_end_ms,omitempty"`
	ContentTokensPerSecond    *float64                  `json:"content_tokens_per_second,omitempty"`
	EventTokensPerSecond      *float64                  `json:"event_tokens_per_second,omitempty"`
	GenerationSample          *metrics.GenerationSample `json:"generation_sample,omitempty"`
	GenerationTokensPerSecond *float64                  `json:"generation_tokens_per_second,omitempty"`
	// ReasoningEffort is the level the client asked for. Omitted when it asked
	// for none - a blank is a different answer from "the lowest level".
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// KeyHint labels the credential that carried this request: the last four
	// characters behind a mask. Omitted when the platform has no key pool or the
	// key was supplied by the environment. Never the credential itself.
	KeyHint             string   `json:"key_hint,omitempty"`
	Scenario            string   `json:"scenario"`
	StartTime           string   `json:"start_time"` // RFC3339
	DurationMs          int64    `json:"duration_ms"`
	InputTokens         int      `json:"input_tokens"`
	PromptTokens        int      `json:"prompt_tokens"`
	OutputTokens        int      `json:"output_tokens"`
	ReasoningTokens     int      `json:"reasoning_tokens,omitempty"`
	CacheReadTokens     int      `json:"cache_read_tokens"`
	CacheCreationTokens int      `json:"cache_creation_tokens"`
	CostUSD             *float64 `json:"cost_usd"`
	CostSource          string   `json:"cost_source,omitempty"`
	PeakMultiplier      float64  `json:"peak_multiplier"`
	DetailsKnown        bool     `json:"details_known"`
	Streaming           bool     `json:"streaming"`
	Attempt             int      `json:"attempt"`
	Success             bool     `json:"success"`
	ErrorMsg            string   `json:"error_msg,omitempty"`
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 || size > 1000 {
		size = 50
	}
	query, err := historyRequestQuery(r, page, size)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Prefer the persistent SQLite history (full dataset) with pagination. When
	// storage is unavailable there is no queryable history to serve: the
	// in-memory ring buffer is a live tail, not a filterable/paginated dataset,
	// so say so instead of shipping a second, divergent query engine.
	if s.storage == nil {
		http.Error(w, "history storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	repo := storage.NewRequests(s.storage)
	records, total, err := repo.Query(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"items": toHistoryEntries(records),
		"total": total,
		"page":  page,
		"size":  size,
	})
}

func (s *Server) handleHistorySummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query, err := historyRequestQuery(r, 1, 1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.storage == nil {
		http.Error(w, "persistent history is unavailable", http.StatusServiceUnavailable)
		return
	}
	summary, err := storage.NewRequests(s.storage).Summary(r.Context(), query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, summary)
}

// handleHistoryChannels reports which upstream channels this instance actually
// recorded, so the history filter can offer real choices instead of the
// gateway's advertised candidate list.
//
// Scoped by platform, because the answer is: only ClinePass publishes a
// channel. Asking for another platform honestly returns none, and the panel
// then shows no channel filter at all rather than an empty dropdown.
func (s *Server) handleHistoryChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.storage == nil {
		http.Error(w, "history storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	channels, err := storage.NewRequests(s.storage).ObservedChannels(r.URL.Query().Get("provider"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"channels": channels})
}

func historyRequestQuery(r *http.Request, page, size int) (storage.RequestQuery, error) {
	values := r.URL.Query()
	q := storage.RequestQuery{
		Page:       page,
		PageSize:   size,
		Search:     strings.TrimSpace(values.Get("search")),
		Model:      strings.TrimSpace(values.Get("model")),
		Provider:   strings.TrimSpace(values.Get("provider")),
		Scenario:   strings.TrimSpace(values.Get("scenario")),
		Channel:    strings.TrimSpace(values.Get("channel")),
		CostSource: strings.TrimSpace(values.Get("cost_source")),
		SortBy:     values.Get("sort"),
		SortOrder:  values.Get("order"),
	}
	if q.CostSource != "" && q.CostSource != storage.CostSourceProvider && q.CostSource != storage.CostSourceEstimated {
		return q, errors.New("invalid cost_source")
	}

	var err error
	if q.Start, err = parseHistoryTime(values.Get("start"), false); err != nil {
		return q, fmt.Errorf("invalid start: %w", err)
	}
	if q.End, err = parseHistoryTime(values.Get("end"), true); err != nil {
		return q, fmt.Errorf("invalid end: %w", err)
	}
	if q.Success, err = parseHistoryBool(values.Get("success")); err != nil {
		return q, fmt.Errorf("invalid success: %w", err)
	}
	if q.Streaming, err = parseHistoryBool(values.Get("streaming")); err != nil {
		return q, fmt.Errorf("invalid streaming: %w", err)
	}
	if q.Start != nil && q.End != nil && !q.Start.Before(*q.End) {
		return q, errors.New("start must be before end")
	}
	return q, nil
}

func parseHistoryTime(value string, endOfDay bool) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return &parsed, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return nil, err
	}
	if endOfDay {
		parsed = parsed.AddDate(0, 0, 1)
	}
	return &parsed, nil
}

func parseHistoryBool(value string) (*bool, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// toHistoryEntries converts request records to the wire format.
func toHistoryEntries(records []history.RequestRecord) []historyEntry {
	out := make([]historyEntry, len(records))
	for i, rec := range records {
		scenario := strings.TrimSpace(rec.Scenario)
		if scenario == "" || strings.EqualFold(scenario, "unknown") {
			scenario = "override"
		}
		entry := historyEntry{
			ID:                        rec.ID,
			Model:                     rec.Model,
			RequestedModel:            rec.RequestedModel,
			Provider:                  rec.Provider,
			UpstreamChannel:           rec.UpstreamChannel,
			UpstreamModel:             rec.UpstreamModel,
			FirstByteMs:               rec.FirstByte.Milliseconds(),
			FirstEventMs:              rec.FirstEventMs,
			FirstContentMs:            rec.FirstContentMs,
			MessageEndMs:              rec.MessageEndMs,
			ContentTokensPerSecond:    rec.ContentTokensPerSecond(),
			EventTokensPerSecond:      rec.EventTokensPerSecond(),
			GenerationSample:          rec.GenerationSample,
			GenerationTokensPerSecond: rec.GenerationTokensPerSecond(),
			ReasoningEffort:           rec.ReasoningEffort,
			KeyHint:                   rec.KeyHint,
			Scenario:                  scenario,
			StartTime:                 rec.StartTime.Format("2006-01-02T15:04:05Z07:00"),
			DurationMs:                rec.Duration.Milliseconds(),
			InputTokens:               rec.InputTokens,
			PromptTokens:              rec.DisplayInputTokens(),
			OutputTokens:              rec.OutputTokens,
			ReasoningTokens:           rec.ReasoningTokens,
			CacheReadTokens:           rec.CacheReadTokens,
			CacheCreationTokens:       rec.CacheCreationTokens,
			DetailsKnown:              rec.DetailsKnown,
			Streaming:                 rec.Streaming,
			Attempt:                   rec.Attempt,
			Success:                   rec.Success,
			ErrorMsg:                  rec.ErrorMsg,
			PeakMultiplier:            rec.PeakMultiplier,
		}
		if rec.CostKnown || rec.CostUSD != 0 {
			entry.CostUSD = &rec.CostUSD
			entry.CostSource = rec.CostSource
		}
		out[i] = entry
	}
	return out
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.cfgMu.RLock()
		cfg := s.cfg
		s.cfgMu.RUnlock()
		writeJSON(w, cfg)

	case http.MethodPost:
		var req struct {
			Autostart *bool `json:"autostart"`
			Notify    *bool `json:"notify"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSettingsError(w, http.StatusBadRequest, "invalid_json", "", false)
			return
		}
		s.cfgMu.Lock()
		defer s.cfgMu.Unlock()
		if req.Autostart != nil {
			set := s.setAutostart
			if set == nil {
				set = func(enabled bool, port int) error {
					if enabled {
						return daemon.EnableAutostart("", port)
					}
					return daemon.DisableAutostart()
				}
			}
			if err := set(*req.Autostart, s.getProxyPort()); err != nil {
				writeSettingsError(w, http.StatusInternalServerError, "autostart_failed", "autostart", false)
				return
			}
			s.cfg.Autostart = *req.Autostart
		}
		if req.Notify != nil {
			s.cfg.Notify = *req.Notify
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// keyMask is the fixed-width placeholder substituted for a *single* secret
// value that leaves the process (settings GET and config export). The frontend
// only needs to know whether a key is set, and sending the real key would leak
// it in the DevTools network tab and to any script on the page.
//
// Key *pools* are not rendered with this mask: one mask for N keys cannot say
// how large the pool is or which row a key occupies, so every pool element gets
// its own maskKeyHint label instead (see anonymizeMap).
const keyMask = "••••••••••••••••"

// keyHintPrefix marks a per-row key label. maskKeyHint leaves the last four
// characters, so a label is stable across page loads without being a credential.
const keyHintPrefix = "••••"

// keySlot is one element of an api_keys array in a settings PATCH. Exactly one
// of Keep and Value is set: Keep re-states an existing credential that the
// client only ever saw as a hint, Value supplies a literal new one.
//
// Hint is required alongside Keep and is verified against the stored key. An
// index alone would silently address the wrong credential if the pool changed
// between the GET that rendered the row and the POST that saved it; the hint
// turns that race into a refusal instead of a rename.
type keySlot struct {
	Keep  *int   `json:"keep,omitempty"`
	Hint  string `json:"hint,omitempty"`
	Value string `json:"value,omitempty"`
}

// stripMaskedKeys removes from a config PATCH any field whose value is still a
// redaction placeholder, recursively, and expands per-row key slots against the
// stored pool. This is a server-side guard against the masked GET response
// being replayed: if the client never edited a key field, its mask must not
// overwrite the real key on disk.
//
// stored is the on-disk config the patch is about to be merged onto, and is the
// only source a keep reference may resolve against. Resolving against the
// running config instead would resolve env-overridden credentials and write
// them to disk, which is exactly what the redaction round trip must never do.
func stripMaskedKeys(patch, stored map[string]json.RawMessage) error {
	for field, raw := range patch {
		// A bare masked string at this level: drop the whole field.
		var asString string
		if json.Unmarshal(raw, &asString) == nil {
			if asString == keyMask {
				delete(patch, field)
			}
			continue
		}

		// An array of key slots, or a legacy array of literal keys.
		var asArray []json.RawMessage
		if json.Unmarshal(raw, &asArray) == nil {
			if field != "api_keys" {
				continue // Routing chains and other arrays are not credential slots.
			}
			keys, err := resolveKeyPatch(json.RawMessage(raw), stored[field], stored["api_key"])
			if err != nil {
				// The slot parser works on elements and cannot name the field,
				// so name it here; the enclosing recursion prefixes the rest of
				// the path. Letting AtField prefix an already-named field would
				// report "cline_pass.api_keys.api_keys".
				var validation *config.ValidationError
				if errors.As(err, &validation) && validation.Field == "" {
					validation.Field = field
					return err
				}
				return config.AtField(field, err)
			}
			if keys == nil {
				// Replaying a folded display must not clear an unfurled file key.
				if field == "api_keys" && len(asArray) > 0 && string(patch["api_key"]) == `""` {
					delete(patch, "api_key")
				}
				delete(patch, field)
				continue
			}
			encoded, err := json.Marshal(keys)
			if err != nil {
				return config.AtField(field, err)
			}
			patch[field] = encoded
			continue
		}

		// A nested object (e.g. "opencode_go"): recurse, then re-encode so the
		// stripped version is what gets merged onto the on-disk config.
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) != nil {
			continue // scalar or shape we don't need to touch
		}
		if len(nested) == 0 {
			continue // An explicit empty object resets that section.
		}
		var storedNested map[string]json.RawMessage
		if len(stored[field]) > 0 {
			_ = json.Unmarshal(stored[field], &storedNested)
		}
		if err := stripMaskedKeys(nested, storedNested); err != nil {
			return config.AtField(field, err)
		}
		if len(nested) == 0 {
			delete(patch, field)
			continue
		}
		if reencoded, err := json.Marshal(nested); err == nil {
			patch[field] = reencoded
		}
	}
	return nil
}

// resolveKeyPatch turns one api_keys array from a PATCH into literal keys.
// A nil result with a nil error means "drop the field": every element was a
// replay of the redacted GET response, so the stored pool is left untouched.
//
// Masked elements that are not expressible as slots are refused rather than
// guessed. Persisting a hint would store a placeholder as a credential, and
// substituting "some stored key" for an ambiguous hint would rotate an account
// the operator did not select.
func resolveKeyPatch(raw, storedRaw, singleRaw json.RawMessage) ([]string, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	if items == nil {
		return nil, nil // explicit null: reset the pool
	}
	if len(items) == 0 {
		return []string{}, nil // explicit empty list: clear the pool
	}
	var stored []string
	if len(storedRaw) > 0 {
		_ = json.Unmarshal(storedRaw, &stored)
	}

	keys := make([]string, 0, len(items))
	keptIndices := make(map[int]bool)
	placeholders := 0
	for _, item := range items {
		var literal string
		if json.Unmarshal(item, &literal) == nil {
			// A legacy client sends plain strings. One that still carries a
			// redaction label is a replay, not a credential.
			if isKeyPlaceholder(literal) {
				placeholders++
				continue
			}
			keys = append(keys, literal)
			continue
		}
		slot, ok := decodeKeySlot(item)
		if !ok {
			return nil, &config.ValidationError{Code: "invalid_type", Err: fmt.Errorf("api_keys entries must be strings or {keep|value} objects")}
		}
		if slot.Keep == nil {
			keys = append(keys, slot.Value)
			continue
		}
		index := *slot.Keep
		var retained string
		if index == -1 {
			_ = json.Unmarshal(singleRaw, &retained)
		} else if index >= 0 && index < len(stored) {
			retained = stored[index]
		}
		if retained == "" {
			return nil, &config.ValidationError{Code: "stale_key_pool", Err: fmt.Errorf("kept key %d is outside the stored pool of %d; reload the settings before saving", index, len(stored))}
		}
		// The label was rendered from the resolved credential, while the file
		// may still hold a ${VAR} placeholder. Resolve before comparing, or
		// every environment-backed key looks stale. The placeholder itself is
		// what gets kept, so a resolved secret never reaches disk.
		if slot.Hint != maskKeyHint(strings.TrimSpace(config.InterpolateEnvVars(retained))) {
			return nil, &config.ValidationError{Code: "stale_key_pool", Err: fmt.Errorf("kept key %d no longer matches the stored credential; reload the settings before saving", index)}
		}
		if !keptIndices[index] {
			keys = append(keys, retained)
			keptIndices[index] = true
		}
		// One displayed row can represent several raw placeholders with equal
		// current values. Keeping that row keeps its whole group; otherwise an
		// unrelated addition would silently discard hidden file configuration.
		// A single-field reference never absorbs the shadowed array: an active
		// array override is exactly why that field was not folded.
		if index >= 0 {
			resolved := strings.TrimSpace(config.InterpolateEnvVars(retained))
			for other, candidate := range stored {
				if !keptIndices[other] && strings.TrimSpace(config.InterpolateEnvVars(candidate)) == resolved {
					keys = append(keys, candidate)
					keptIndices[other] = true
				}
			}
		}
	}
	if placeholders > 0 {
		if len(keys) > 0 {
			return nil, &config.ValidationError{Code: "masked_keys_mixed", Err: fmt.Errorf("api_keys mixes redacted and new values; keep the stored key by index instead of resending its label")}
		}
		return nil, nil // every element was a replay: leave the stored pool alone
	}
	return keys, nil
}

// isKeyPlaceholder reports whether a value is a redaction label rather than a
// credential. Both the legacy single mask and the per-row hints are labels.
func isKeyPlaceholder(value string) bool {
	return value == keyMask || strings.HasPrefix(value, keyHintPrefix)
}

func decodeKeySlot(item json.RawMessage) (keySlot, bool) {
	var slot keySlot
	if json.Unmarshal(item, &slot) != nil {
		return keySlot{}, false
	}
	if (slot.Keep == nil) == (slot.Value == "") {
		return keySlot{}, false // exactly one of the two forms
	}
	return slot, true
}

func (s *Server) handleProxyStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.proxyRunning.Load() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.startProxy != nil {
		if err := s.startProxy(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProxyStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.proxyRunning.Load() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.stopProxy != nil {
		if err := s.stopProxy(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// keyOriginView renders each platform's per-row key origin for the settings
// response, keyed by the config block name ("" for the global pool).
//
// It is a sibling field rather than part of the config object: origins are
// runtime knowledge, not configuration, and putting them inside the config
// would let a client echo them back on save.
func keyOriginView(cfg *config.Config) map[string][]string {
	if cfg == nil {
		return nil
	}
	view := map[string][]string{}
	for block, pool := range cfg.KeyPools() {
		if len(pool.Origins) != len(pool.Keys) || len(pool.Keys) == 0 {
			continue
		}
		origins := make([]string, len(pool.Origins))
		for i, origin := range pool.Origins {
			origins[i] = string(origin)
		}
		view[block] = origins
	}
	return view
}

// writeSettingsPayload writes the redacted config with the key-origin map
// merged in as an extra top-level field.
//
// The config is flattened rather than nested so the response keeps the shape
// every existing client already reads: a settings save sends back a patch built
// from these same top-level keys, and wrapping them would silently stop that
// patch from addressing anything.
func writeSettingsPayload(w http.ResponseWriter, cfg *config.Config, origins map[string][]string, indices map[string][]*int) {
	data, err := json.Marshal(cfg)
	if err != nil {
		writeSettingsError(w, http.StatusInternalServerError, "redaction_failed", "", false)
		return
	}
	var flat map[string]json.RawMessage
	if err := json.Unmarshal(data, &flat); err != nil {
		writeSettingsError(w, http.StatusInternalServerError, "redaction_failed", "", false)
		return
	}
	if len(origins) > 0 {
		encoded, err := json.Marshal(origins)
		if err == nil {
			flat["key_origins"] = encoded
		}
	}
	if encoded, err := json.Marshal(indices); err == nil {
		flat["key_indices"] = encoded
	}
	writeJSON(w, flat)
}

func (s *Server) handleProxyConfig(w http.ResponseWriter, r *http.Request) {
	if s.atomicCfg == nil {
		writeSettingsError(w, http.StatusServiceUnavailable, "config_unavailable", "", false)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Never ship raw API keys to the browser: they would be visible in the
		// DevTools network tab and readable by any script on the page. The
		// settings form only needs to know whether a key is set, so send a
		// fixed-width mask instead. Saving an unchanged masked field is a no-op
		// because the frontend only patches fields the user actually edited.
		current := s.atomicCfg.Get()
		redacted, err := anonymizeConfig(current)
		if err != nil {
			// Fail closed: an unredactable config is never sent unredacted.
			writeSettingsError(w, http.StatusInternalServerError, "redaction_failed", "", false)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeSettingsPayload(w, redacted, keyOriginView(current), current.KeyPoolIndices())

	case http.MethodPost:
		// Decode only the fields the client sent (partial update).
		var patch map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeSettingsError(w, http.StatusBadRequest, "invalid_json", "", false)
			return
		}

		if _, err := s.updateProxyConfig(patch, true); err != nil {
			writeConfigError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type catalogLockResponse struct {
	SyncedAt   *time.Time `json:"synced_at,omitempty"`
	SHA256     string     `json:"sha256,omitempty"`
	Bytes      int64      `json:"bytes,omitempty"`
	TTLHours   int        `json:"ttl_hours,omitempty"`
	AgeSeconds int64      `json:"age_seconds"`
	Synced     bool       `json:"synced"`
}

const maxTestRequestBody = 1 << 20

// handleTestSend proxies a chat request to the proxy server and streams the
// response back. This avoids CORS issues that would arise from the browser
// calling the proxy port directly.
func (s *Server) handleTestSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	proxyPort := s.getProxyPort()
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d/v1/messages", proxyPort)

	r.Body = http.MaxBytesReader(w, r.Body, maxTestRequestBody)
	defer func() { _ = r.Body.Close() }()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Forward to the proxy.
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, proxyURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to create proxy request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Copy status code and headers.
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	written, err := io.Copy(w, resp.Body)
	if err != nil {
		logger := s.logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Error("failed to stream proxy response", "err", err, "bytes_written", written)
	}
}

// catalogModelEntry is one row of the model browser: enough for a picker to
// sort, group and describe a model without a second request per row.
type catalogModelEntry struct {
	ID            string  `json:"id"`
	Name          string  `json:"name,omitempty"`
	Provider      string  `json:"provider"`
	ContextWindow int64   `json:"context_window,omitempty"`
	MaxOutput     int64   `json:"max_output,omitempty"`
	InputPerM     float64 `json:"input_per_m,omitempty"`
	OutputPerM    float64 `json:"output_per_m,omitempty"`
	ToolCall      bool    `json:"tool_call,omitempty"`
	Vision        bool    `json:"vision,omitempty"`
	Reasoning     bool    `json:"reasoning,omitempty"`
	// RoutesTo names the model an alias resolves to. An alias is a requestable
	// id - a client sends it and the router maps it - but it is not a model the
	// catalog knows, so it has no context window or rates of its own. Saying
	// what it points at is the difference between "this row is missing data"
	// and "this row is an alias".
	RoutesTo string `json:"routes_to,omitempty"`
}

// handleCatalogModels reports the models this instance can actually send to,
// grouped by platform.
//
// The source is the proxy's own /v1/models rather than the catalog file. That
// listing is the only answer that already applies the routing rules: it hides
// platforms outside the active site, drops catalog entries a platform's live
// roster has retired, and adds rosters the catalog has never carried
// (CommandCode is absent from models.dev entirely). Reading the catalog file
// directly would offer models that fail on send, which is the bug this endpoint
// exists to prevent.
func (s *Server) handleCatalogModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wanted := strings.TrimSpace(r.URL.Query().Get("provider"))
	scopeAll := strings.TrimSpace(r.URL.Query().Get("scope")) == "all"
	if scopeAll {
		// The fallback picker asks a different question than the test picker:
		// "which platforms could I fall back to", not "what is routed right
		// now". /v1/models answers the second - it hides platforms outside the
		// active site - so using it here offered only the current platform and
		// made a cross-platform fallback impossible to configure.
		s.serveAllReachableModels(w, wanted)
		return
	}

	resp, err := s.fetchProxyModels(r.Context())
	if err != nil {
		// A failed listing is not an empty one. Reporting 200 with no models
		// would read as "this instance serves nothing", which is a different
		// and alarming claim; the dashboard needs to say the lookup failed.
		http.Error(w, "proxy model listing unavailable", http.StatusServiceUnavailable)
		return
	}

	entries := make([]catalogModelEntry, 0, len(resp.Data))
	// /v1/models answers "what may a client request", which is not the same as
	// "what is this model" - it carries no context window and no rates. Those
	// come from the catalog and the platform price tables, joined by the id the
	// listing returned. Both are best-effort: a model the catalog has never
	// heard of still lists, just without the detail.
	cat := s.catalogIndex()
	for _, m := range resp.Data {
		if wanted != "" && site.Normalize(m.OwnedBy) != site.Normalize(wanted) {
			continue
		}
		e := catalogModelEntry{ID: m.ID, Name: m.Name, Provider: m.OwnedBy}
		if e.Name == "" {
			e.Name = m.ID
		}
		if cat != nil {
			if model, ok := catalogModelFor(cat, m.ID, m.OwnedBy); ok {
				e.Name = model.DisplayName()
				e.ToolCall = model.ToolCall
				e.Vision = model.SupportsVision()
				e.Reasoning = model.Reasoning
				if model.Limit != nil {
					e.ContextWindow = model.Limit.Context
					e.MaxOutput = model.Limit.Output
				}
			}
		}
		// cfg.Models is keyed by scenario, so its entries name routing slots, not
		// models a client may request. Only model_overrides produces a requestable
		// alias; a scenario key that leaked into this list would look like a
		// model and fail when sent.
		if s.atomicCfg != nil {
			if cfg := s.atomicCfg.Get(); cfg != nil {
				if mc, ok := cfg.ModelOverrides[m.ID]; ok {
					e.RoutesTo = mc.ModelID
				} else if slices.Contains(config.CostScenarioNames, m.ID) {
					continue
				}
			}
		}
		if in, out, _, _, ok := storage.PriceForProviderModel(m.OwnedBy, m.ID, 0); ok {
			e.InputPerM = in
			e.OutputPerM = out
		}
		entries = append(entries, e)
	}

	writeJSON(w, map[string]any{
		"models": entries,
		"scope":  s.activeSiteID(),
		"total":  len(entries),
	})
}

// catalogIndex returns the parsed catalog, loading it on first use.
//
// Nil is a valid answer: a missing or unreadable catalog leaves the listing
// without detail rather than failing it, because the ids alone are still what
// the picker needs. A load failure is not cached, so the next call retries.
func (s *Server) catalogIndex() *catalog.IndexedCatalog {
	s.catalogModelsMu.Lock()
	defer s.catalogModelsMu.Unlock()
	if s.catalogModelsIndex != nil {
		return s.catalogModelsIndex
	}
	if s.catalogDir == "" {
		return nil
	}
	cat, err := catalog.Load(filepath.Join(s.catalogDir, "catalog.json"))
	if err != nil {
		return nil
	}
	s.catalogModelsIndex = cat
	return cat
}

// invalidateCatalogIndex drops the cached catalog after a re-sync.
func (s *Server) invalidateCatalogIndex() {
	s.catalogModelsMu.Lock()
	s.catalogModelsIndex = nil
	s.catalogModelsMu.Unlock()
}

// catalogModelFor resolves a served model id to its catalog entry.
//
// The catalog is keyed by models.dev's provider-qualified id, and a platform's
// own roster does not always use that provider name. ClinePass's gateway serves
// `cline-pass/deepseek-v4.1-flash-1017`, which the catalog carries only as
// `deepseek/deepseek-v4.1-flash-1017` - the platform is a reseller, so nothing
// under `cline-pass/` describes it. Without a fallback that row lists with an
// id and a price but no context window, no capability flags and no display
// name, which reads as missing data rather than as a model the catalog does not
// file under that provider.
//
// The fallback matches the bare model name across every provider. That is only
// safe because the catalog does not disagree with itself: across all 602 bare
// names, no two providers describe the same name differently. A bare name that
// did carry two specs would still resolve to one of them, so this degrades to
// the old behaviour rather than to a wrong one. A candidate from the model's
// own provider is preferred when one exists, so the fallback cannot override a
// provider-specific entry.
func catalogModelFor(cat *catalog.IndexedCatalog, id, provider string) (catalog.Model, bool) {
	if model, ok := cat.Models[id]; ok {
		return model, true
	}
	bare := id
	if slash := strings.LastIndex(id, "/"); slash >= 0 {
		bare = id[slash+1:]
	}
	qualified := provider + "/" + bare
	if provider != "" {
		if model, ok := cat.Models[qualified]; ok {
			return model, true
		}
	}
	for key, model := range cat.Models {
		if key == bare || strings.HasSuffix(key, "/"+bare) {
			return model, true
		}
	}
	return catalog.Model{}, false
}

// proxyModel is the shape /v1/models returns for one model.
type proxyModel struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

type proxyModelListing struct {
	Data []proxyModel `json:"data"`
}

// serveAllReachableModels lists every model on every platform that holds a
// credential, ignoring the active site.
//
// Only platforms with a key are included: a fallback there could never be sent,
// and offering it would produce a chain that reads as protected and fails on the
// attempt that needed it.
func (s *Server) serveAllReachableModels(w http.ResponseWriter, wanted string) {
	cfg := s.configSnapshot()
	if cfg == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	cat := s.catalogIndex()
	entries := []catalogModelEntry{}
	for _, descriptor := range site.All() {
		if wanted != "" && site.Normalize(descriptor.ID) != site.Normalize(wanted) {
			continue
		}
		if len(cfg.ProviderAPIKeys(descriptor.ID)) == 0 {
			continue
		}
		if cat == nil {
			continue
		}
		for _, resolved := range cat.ListProviderModels(descriptor.ID) {
			// The model_id a fallback entry stores is the name within its
			// platform, which is not the wire id: WireModelID strips the provider
			// for most platforms but keeps it for ClinePass, while a chain entry
			// carries provider and model_id separately. Taking the wire form here
			// produced ids that did not match what the chain expects, so a
			// selected model was written back as a different model.
			id := resolved.ModelID
			if key := resolved.CanonicalName; key != "" {
				if idx := strings.IndexByte(key, '/'); idx >= 0 {
					id = key[idx+1:]
				} else {
					id = key
				}
			}
			if id == "" {
				continue
			}
			e := catalogModelEntry{
				ID:            id,
				Name:          resolved.DisplayName,
				Provider:      descriptor.ID,
				ContextWindow: resolved.ContextWindow,
				MaxOutput:     resolved.MaxOutputTokens,
				ToolCall:      resolved.Tools,
				Vision:        resolved.Vision,
				Reasoning:     resolved.Reasoning,
			}
			if e.Name == "" {
				e.Name = id
			}
			// The price tables match by substring against the id the platform
			// uses, and ClinePass's rules are written with the pool prefix
			// ("cline-pass/deepseek-v4.1-flash"). Look the price up with the
			// canonical key while still handing the client the bare model_id,
			// or the row would show no rate even though one is published.
			priceKey := resolved.CanonicalName
			if priceKey == "" {
				priceKey = id
			}
			if in, out, _, _, ok := storage.PriceForProviderModel(descriptor.ID, priceKey, 0); ok {
				e.InputPerM = in
				e.OutputPerM = out
			}
			entries = append(entries, e)
		}
	}
	writeJSON(w, map[string]any{
		"models": entries,
		"scope":  "",
		"total":  len(entries),
	})
}

// configSnapshot returns the live config, or nil when the server has none.
func (s *Server) configSnapshot() *config.Config {
	if s.atomicCfg == nil {
		return nil
	}
	return s.atomicCfg.Get()
}

// fetchProxyModels asks the running proxy for its model list.
func (s *Server) fetchProxyModels(ctx context.Context) (*proxyModelListing, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/models", s.getProxyPort())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model listing returned HTTP %d", resp.StatusCode)
	}
	var out proxyModelListing
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("parse model listing: %w", err)
	}
	return &out, nil
}

// activeSiteID reports the platform routing is scoped to, or "" when unscoped.
func (s *Server) activeSiteID() string {
	if s.atomicCfg == nil {
		return ""
	}
	cfg := s.atomicCfg.Get()
	if cfg == nil {
		return ""
	}
	return site.Normalize(cfg.ActiveSite)
}

func (s *Server) handleCatalogLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lock, err := catalog.ReadLock(s.catalogDir)
	if err != nil {
		writeJSON(w, catalogLockResponse{Synced: false, AgeSeconds: -1})
		return
	}

	age := time.Since(lock.SyncedAt)
	resp := catalogLockResponse{
		SyncedAt:   &lock.SyncedAt,
		SHA256:     lock.SHA256,
		Bytes:      lock.Bytes,
		TTLHours:   lock.TTLHours,
		AgeSeconds: int64(age.Seconds()),
		Synced:     true,
	}
	writeJSON(w, resp)
}

// handlePrices reports the provenance of each platform's price table: how many
// rules are installed, how many come from the build-time seed, and when the
// live refresh last succeeded. The dashboard uses it to say whether a cost
// figure was computed from current published rates or from a snapshot that may
// have moved - a distinction the project has needed since its own seed was
// found to be stale.
func (s *Server) handlePrices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"tables":       storage.PriceTables(),
		"generated_at": time.Now().UTC(),
	})
}

func (s *Server) handleCatalogSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.catalogSourceURL == "" || s.catalogDir == "" {
		http.Error(w, "catalog sync is not configured", http.StatusServiceUnavailable)
		return
	}

	lock, err := catalog.Sync(r.Context(), s.catalogSourceURL, s.catalogDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("catalog sync failed: %v", err), http.StatusInternalServerError)
		return
	}
	// The old file's parse is still cached; a reader who re-synced expects the
	// model list to reflect what the sync just fetched.
	s.invalidateCatalogIndex()

	age := time.Since(lock.SyncedAt)
	writeJSON(w, catalogLockResponse{
		SyncedAt:   &lock.SyncedAt,
		SHA256:     lock.SHA256,
		Bytes:      lock.Bytes,
		TTLHours:   lock.TTLHours,
		AgeSeconds: int64(age.Seconds()),
		Synced:     true,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// securityHeadersMiddleware adds security headers to all responses.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME-type sniffing.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Restrict all resources to same-origin (all assets bundled in binary).
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}
