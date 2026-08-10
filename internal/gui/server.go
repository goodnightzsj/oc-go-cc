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
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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
	hist              *history.History
	met               *metrics.Metrics
	atomicCfg         *config.AtomicConfig
	cfg               Config
	cfgMu             sync.RWMutex
	proxyRunning      atomic.Bool
	connectedExisting atomic.Bool
	proxyPort         int
	guiPort           atomic.Int32
	startProxy        func() error
	stopProxy         func() error
	catalogDir        string
	catalogSourceURL  string
	srv               *http.Server
	logger            *slog.Logger
	catalogMu         sync.Mutex

	storage *storage.Database
}

// Options configures the GUI server.
type Options struct {
	History          *history.History
	Metrics          *metrics.Metrics
	AtomicConfig     *config.AtomicConfig
	ProxyPort        int
	StartProxy       func() error
	StopProxy        func() error
	CatalogDir       string
	CatalogSourceURL string
	Logger           *slog.Logger
	Storage          *storage.Database
}

// New creates a new GUI server.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Server{
		hist:             opts.History,
		met:              opts.Metrics,
		atomicCfg:        opts.AtomicConfig,
		proxyPort:        opts.ProxyPort,
		startProxy:       opts.StartProxy,
		stopProxy:        opts.StopProxy,
		catalogDir:       opts.CatalogDir,
		catalogSourceURL: opts.CatalogSourceURL,
		logger:           opts.Logger,

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

// Start starts the embedded HTTP server on port 3445 and returns
// the URL that the webview should load. If another routatic-proxy instance
// is using that port, it is killed before binding.
func (s *Server) Start(ctx context.Context) (string, error) {
	s.guiPort.Store(3445)
	// Allow GUI port override via environment variable.
	if envPort := os.Getenv("ROUTATIC_PROXY_GUI_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 && p < 65536 {
			s.guiPort.Store(int32(p))
		}
	}
	// Ensure port is free, killing any existing routatic-proxy GUI.
	if err := s.ensurePortAvailable(); err != nil {
		return "", fmt.Errorf("gui port check: %w", err)
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
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/proxy/config", s.handleProxyConfig)
	mux.HandleFunc("/api/proxy/start", s.handleProxyStart)
	mux.HandleFunc("/api/proxy/stop", s.handleProxyStop)
	mux.HandleFunc("/api/catalog/lock", s.handleCatalogLock)
	mux.HandleFunc("/api/catalog/sync", s.handleCatalogSync)
	mux.HandleFunc("/api/test/send", s.handleTestSend)

	// New endpoints for advanced GUI features

	mux.HandleFunc("/api/config/export", s.handleConfigExport)
	mux.HandleFunc("/api/config/import", s.handleConfigImport)
	mux.HandleFunc("/api/perf/models", s.handlePerformance)
	mux.HandleFunc("/api/perf/aggregate", s.handlePerformanceAggregate)
	mux.HandleFunc("/api/catalog/stats", s.handleCatalogStats)

	// Analytics (only when SQLite storage is available)
	if s.storage != nil {
		ah := NewAnalyticsHandler(s.storage)
		mux.HandleFunc("/api/analytics/summary", ah.Summary)
		mux.HandleFunc("/api/analytics/tokens/trend", ah.TokenTrend)
		mux.HandleFunc("/api/analytics/latency", ah.LatencyStats)
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
			fmt.Printf("Port %d in use by another app, trying port %d for GUI\n", p, p+1)
		}
	}
	if ln == nil {
		return "", fmt.Errorf("gui server listen: no available port in range %d-%d", startPort, startPort+9)
	}

	// Wrap with security headers middleware.
	s.srv = &http.Server{Handler: securityHeadersMiddleware(mux)}
	go func() {
		if srvErr := s.srv.Serve(ln); srvErr != nil && srvErr != http.ErrServerClosed {
			s.logger.Error("gui server error", "err", srvErr)
		}
	}()

	go func() {
		<-ctx.Done()
		_ = s.srv.Close()
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
	return s.srv.Shutdown(ctx)
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
	writeJSON(w, resp)
}

type historyEntry struct {
	ID                  string   `json:"id"`
	Model               string   `json:"model"`
	Provider            string   `json:"provider"`
	Scenario            string   `json:"scenario"`
	StartTime           string   `json:"start_time"` // RFC3339
	DurationMs          int64    `json:"duration_ms"`
	InputTokens         int      `json:"input_tokens"`
	PromptTokens        int      `json:"prompt_tokens"`
	OutputTokens        int      `json:"output_tokens"`
	CacheReadTokens     int      `json:"cache_read_tokens"`
	CacheCreationTokens int      `json:"cache_creation_tokens"`
	CostUSD             *float64 `json:"cost_usd"`
	CostSource          string   `json:"cost_source,omitempty"`
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

	// Prefer the persistent SQLite history (full dataset) with pagination;
	// fall back to the in-memory ring buffer only when storage is unavailable.
	if s.storage != nil {
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
		return
	}

	if s.hist == nil {
		writeJSON(w, map[string]any{"items": []historyEntry{}, "total": 0, "page": page, "size": size})
		return
	}
	records, total := filterMemoryHistory(s.hist.Last(0), query)
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
	summary, err := storage.NewRequests(s.storage).Summary(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, summary)
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

func filterMemoryHistory(records []history.RequestRecord, q storage.RequestQuery) ([]history.RequestRecord, int64) {
	search := strings.ToLower(strings.TrimSpace(q.Search))
	filtered := make([]history.RequestRecord, 0, len(records))
	for _, rec := range records {
		if search != "" && !strings.Contains(strings.ToLower(strings.Join([]string{
			rec.ID, rec.Model, rec.Provider, rec.Scenario, rec.ErrorMsg,
		}, "\n")), search) {
			continue
		}
		if (q.Model != "" && rec.Model != q.Model) ||
			(q.Provider != "" && rec.Provider != q.Provider) ||
			(q.Scenario != "" && rec.Scenario != q.Scenario) ||
			(q.CostSource != "" && rec.CostSource != q.CostSource) {
			continue
		}
		if (q.Start != nil && rec.StartTime.Before(*q.Start)) ||
			(q.End != nil && !rec.StartTime.Before(*q.End)) {
			continue
		}
		if (q.Success != nil && rec.Success != *q.Success) ||
			(q.Streaming != nil && rec.Streaming != *q.Streaming) {
			continue
		}
		filtered = append(filtered, rec)
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		cmp := compareHistoryRecord(filtered[i], filtered[j], q.SortBy)
		if cmp == 0 && q.SortBy != "start_time" {
			return filtered[i].StartTime.After(filtered[j].StartTime)
		}
		if strings.EqualFold(q.SortOrder, "asc") {
			return cmp < 0
		}
		return cmp > 0
	})

	total := int64(len(filtered))
	start := (q.Page - 1) * q.PageSize
	if start >= len(filtered) {
		return []history.RequestRecord{}, total
	}
	end := min(start+q.PageSize, len(filtered))
	return filtered[start:end], total
}

func compareHistoryRecord(a, b history.RequestRecord, field string) int {
	switch field {
	case "model":
		return strings.Compare(strings.ToLower(a.Model), strings.ToLower(b.Model))
	case "provider":
		return strings.Compare(strings.ToLower(a.Provider), strings.ToLower(b.Provider))
	case "scenario":
		return strings.Compare(strings.ToLower(a.Scenario), strings.ToLower(b.Scenario))
	case "prompt_tokens":
		return a.DisplayInputTokens() - b.DisplayInputTokens()
	case "output_tokens":
		return a.OutputTokens - b.OutputTokens
	case "cost_usd":
		return floatCompare(a.CostUSD, b.CostUSD)
	case "duration_ms":
		return int(a.Duration.Milliseconds() - b.Duration.Milliseconds())
	case "success":
		return boolToCompare(a.Success, b.Success)
	case "streaming":
		return boolToCompare(a.Streaming, b.Streaming)
	default:
		if a.StartTime.Before(b.StartTime) {
			return -1
		}
		if a.StartTime.After(b.StartTime) {
			return 1
		}
		return 0
	}
}

func floatCompare(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func boolToCompare(a, b bool) int {
	if a == b {
		return 0
	}
	if a {
		return 1
	}
	return -1
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
			ID:                  rec.ID,
			Model:               rec.Model,
			Provider:            rec.Provider,
			Scenario:            scenario,
			StartTime:           rec.StartTime.Format("2006-01-02T15:04:05Z07:00"),
			DurationMs:          rec.Duration.Milliseconds(),
			InputTokens:         rec.InputTokens,
			PromptTokens:        rec.DisplayInputTokens(),
			OutputTokens:        rec.OutputTokens,
			CacheReadTokens:     rec.CacheReadTokens,
			CacheCreationTokens: rec.CacheCreationTokens,
			DetailsKnown:        rec.DetailsKnown,
			Streaming:           rec.Streaming,
			Attempt:             rec.Attempt,
			Success:             rec.Success,
			ErrorMsg:            rec.ErrorMsg,
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
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.cfgMu.Lock()
		if req.Autostart != nil {
			s.cfg.Autostart = *req.Autostart
			if *req.Autostart {
				_ = daemon.EnableAutostart("", s.getProxyPort())
			} else {
				_ = daemon.DisableAutostart()
			}
		}
		if req.Notify != nil {
			s.cfg.Notify = *req.Notify
		}
		s.cfgMu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// redactConfigKeys returns a shallow clone of cfg with all API key fields
// replaced by a fixed-width mask. The frontend settings form only needs to know
// whether a key is set, not the key itself. Sending the actual keys to the
// browser would leak them in the DevTools network tab and make them readable by
// any script on the page. When the user saves without editing a masked field,
// the frontend omits that field from the PATCH so the server never receives the
// mask, leaving the real key on disk intact.
func redactConfigKeys(cfg *config.Config) *config.Config {
	out := *cfg // shallow copy

	if out.APIKey != "" {
		out.APIKey = keyMask
	}
	if len(out.APIKeys) > 0 {
		out.APIKeys = make([]string, len(out.APIKeys))
		for i := range out.APIKeys {
			out.APIKeys[i] = keyMask
		}
	}
	if out.OpenCodeGo.APIKey != "" {
		out.OpenCodeGo.APIKey = keyMask
	}
	if len(out.OpenCodeGo.APIKeys) > 0 {
		out.OpenCodeGo.APIKeys = make([]string, len(out.OpenCodeGo.APIKeys))
		for i := range out.OpenCodeGo.APIKeys {
			out.OpenCodeGo.APIKeys[i] = keyMask
		}
	}
	if out.OpenCodeZen.APIKey != "" {
		out.OpenCodeZen.APIKey = keyMask
	}
	if len(out.OpenCodeZen.APIKeys) > 0 {
		out.OpenCodeZen.APIKeys = make([]string, len(out.OpenCodeZen.APIKeys))
		for i := range out.OpenCodeZen.APIKeys {
			out.OpenCodeZen.APIKeys[i] = keyMask
		}
	}
	if out.AWSBedrock.APIKey != "" {
		out.AWSBedrock.APIKey = keyMask
	}
	if len(out.AWSBedrock.APIKeys) > 0 {
		out.AWSBedrock.APIKeys = make([]string, len(out.AWSBedrock.APIKeys))
		for i := range out.AWSBedrock.APIKeys {
			out.AWSBedrock.APIKeys[i] = keyMask
		}
	}
	if out.OpenRouter.APIKey != "" {
		out.OpenRouter.APIKey = keyMask
	}
	if len(out.OpenRouter.APIKeys) > 0 {
		out.OpenRouter.APIKeys = make([]string, len(out.OpenRouter.APIKeys))
		for i := range out.OpenRouter.APIKeys {
			out.OpenRouter.APIKeys[i] = keyMask
		}
	}
	return &out
}

// maskForKeys is the exact mask redactConfigKeys substitutes for real keys.
const keyMask = "••••••••••••••••"

// stripMaskedKeys removes from a config PATCH any field whose value is still
// the keyMask placeholder, recursively. This is a server-side guard against the
// masked GET response being replayed: if the client never edited a key field,
// its mask must not overwrite the real key on disk.
func stripMaskedKeys(patch map[string]json.RawMessage) {
	for field, raw := range patch {
		// A bare masked string at this level: drop the whole field.
		var asString string
		if json.Unmarshal(raw, &asString) == nil {
			if asString == keyMask {
				delete(patch, field)
			}
			continue
		}

		// An array whose every element is the mask: drop the whole field.
		var asArray []string
		if json.Unmarshal(raw, &asArray) == nil {
			if len(asArray) > 0 && allMasked(asArray) {
				delete(patch, field)
			}
			continue
		}

		// A nested object (e.g. "opencode_go"): recurse, then re-encode so the
		// stripped version is what gets merged onto the on-disk config.
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) != nil {
			continue // scalar or shape we don't need to touch
		}
		stripMaskedKeys(nested)
		if len(nested) == 0 {
			delete(patch, field)
			continue
		}
		if reencoded, err := json.Marshal(nested); err == nil {
			patch[field] = reencoded
		}
	}
}

func allMasked(values []string) bool {
	for _, v := range values {
		if v != keyMask {
			return false
		}
	}
	return true
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

func (s *Server) handleProxyConfig(w http.ResponseWriter, r *http.Request) {
	if s.atomicCfg == nil {
		http.Error(w, "proxy config not available", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Never ship raw API keys to the browser: they would be visible in the
		// DevTools network tab and readable by any script on the page. The
		// settings form only needs to know whether a key is set, so send a
		// fixed-width mask instead. Saving an unchanged masked field is a no-op
		// because the frontend only patches fields the user actually edited.
		writeJSON(w, redactConfigKeys(s.atomicCfg.Get()))

	case http.MethodPost:
		// Decode only the fields the client sent (partial update).
		var patch map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, fmt.Sprintf("invalid config format: %v", err), http.StatusBadRequest)
			return
		}

		// Drop any key field whose value is still the mask sent by GET. The
		// frontend normally omits untouched fields, but a browser autofill or a
		// client replaying a GET response would otherwise overwrite the real key
		// on disk with bullet characters.
		stripMaskedKeys(patch)

		if err := applyConfigPatch(s.atomicCfg.Path(), patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Reload configuration atomically so the running proxy picks up changes.
		if err := s.atomicCfg.Reload(); err != nil {
			http.Error(w, fmt.Sprintf("failed to reload config: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// applyConfigPatch merges a partial update into the config file on disk.
//
// The merge happens on the file's *raw* JSON, never on a parsed config.Config.
// config.LoadFromPath expands ${VAR} references, applies environment overrides
// and fills in defaults; round-tripping through it would rewrite the file with
// every secret resolved to plaintext and every default materialised, silently
// destroying the user's ${VAR} indirection.
func applyConfigPatch(configPath string, patch map[string]json.RawMessage) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read current config: %w", err)
	}

	var merged map[string]json.RawMessage
	if err := json.Unmarshal(raw, &merged); err != nil {
		return fmt.Errorf("failed to parse current config: %w", err)
	}
	if merged == nil {
		merged = map[string]json.RawMessage{}
	}
	for field, value := range patch {
		merged[field] = value
	}

	// Validate the result by loading it the way the proxy will, without letting
	// that normalised form reach the file.
	if err := validateMergedConfig(merged); err != nil {
		return err
	}

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// validateMergedConfig checks the essential fields the proxy cannot start
// without. It works on the raw merged JSON so that unset fields stay unset.
func validateMergedConfig(merged map[string]json.RawMessage) error {
	var probe struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	blob, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("failed to re-encode config: %w", err)
	}
	if err := json.Unmarshal(blob, &probe); err != nil {
		return fmt.Errorf("invalid config format: %w", err)
	}
	if probe.Host == "" {
		return errors.New("host is required")
	}
	if probe.Port < 1 || probe.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	return nil
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

func (s *Server) handleCatalogSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.catalogSourceURL == "" || s.catalogDir == "" {
		http.Error(w, "catalog sync is not configured", http.StatusServiceUnavailable)
		return
	}

	// Serialize manual syncs so the lock file and on-disk catalog stay consistent.
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()

	lock, err := catalog.Sync(s.catalogSourceURL, s.catalogDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("catalog sync failed: %v", err), http.StatusInternalServerError)
		return
	}

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

func (s *Server) handleCatalogStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.storage == nil {
		writeJSON(w, map[string]any{
			"available": false,
			"error":     "storage not configured",
		})
		return
	}

	repo := storage.NewCatalogRepo(s.storage)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	idx, err := repo.Load(ctx)
	if err != nil {
		writeJSON(w, map[string]any{
			"available": false,
			"error":     err.Error(),
		})
		return
	}

	lastSync, _ := repo.LastSync(ctx)

	providersByEnabled := make(map[string]int)
	for _, p := range idx.Providers {
		enabled := "disabled"
		if p.Enabled != nil && *p.Enabled {
			enabled = "enabled"
		}
		providersByEnabled[enabled]++
	}

	modelsByProvider := make(map[string]int)
	for prov := range idx.Providers {
		modelsByProvider[prov] = len(idx.ProviderModels[prov])
	}

	totalModels := len(idx.Models)
	modelsWithTools := 0
	modelsWithVision := 0
	modelsWithReasoning := 0
	for _, m := range idx.Models {
		if m.ToolCall {
			modelsWithTools++
		}
		if m.Vision {
			modelsWithVision++
		}
		if m.Reasoning {
			modelsWithReasoning++
		}
	}

	resp := map[string]any{
		"available":             true,
		"last_sync":             lastSync,
		"total_providers":       len(idx.Providers),
		"providers_enabled":     providersByEnabled["enabled"],
		"providers_disabled":    providersByEnabled["disabled"],
		"total_models":          totalModels,
		"models_with_tools":     modelsWithTools,
		"models_with_vision":    modelsWithVision,
		"models_with_reasoning": modelsWithReasoning,
		"models_by_provider":    modelsByProvider,
	}

	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ensurePortAvailable checks if an existing routatic-proxy GUI is running on
// the configured port and kills it if found. It does not test-bind — the caller
// handles binding and will retry with higher ports on failure.
func (s *Server) ensurePortAvailable() error {
	client := &http.Client{Timeout: 2 * time.Second}
	startPort := int(s.guiPort.Load())

	for p := startPort; p < startPort+10; p++ {
		// Check if this port responds to the routatic-proxy metrics endpoint
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/metrics", p))
		if err != nil {
			// No HTTP response → port is free or another app
			continue
		}

		var m struct {
			ProxyRunning bool `json:"proxy_running"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&m)
		_ = resp.Body.Close()

		if decodeErr == nil {
			// Our GUI — kill the existing instance
			s.logger.Info("killing existing routatic-proxy GUI on port", "port", p)
			if err := s.killProcessOnPort(p); err != nil {
				return fmt.Errorf("failed to kill existing instance: %w", err)
			}
			s.guiPort.Store(int32(p))
			return nil
		}

		// Responded but not our metrics format → another app
		if p == startPort {
			fmt.Printf("Port %d in use by another app, trying port %d for GUI\n", p, p+1)
		}
	}

	return nil
}

// killProcessOnPort terminates the process listening on the given port.
// Platform-aware: uses lsof+ps on Unix, netstat+tasklist on Windows.
func (s *Server) killProcessOnPort(port int) error {
	pids, err := s.findPIDsOnPort(port)
	if err != nil || len(pids) == 0 {
		return nil
	}

	killed := false
	for _, pid := range pids {
		if !s.isRoutaticProxyProcess(pid) {
			continue
		}
		s.logger.Info("terminating routatic-proxy process", "pid", pid)
		if s.killProcess(pid) {
			killed = true
		}
	}

	if !killed {
		return fmt.Errorf("no routatic-proxy process found on port %d", port)
	}

	// Wait for port to be released
	for i := 0; i < 10; i++ {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return nil
		}
		_ = conn.Close()
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("port %d not released after killing process", port)
}

// findPIDsOnPort returns PIDs listening on the given port.
func (s *Server) findPIDsOnPort(port int) ([]int, error) {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("cmd", "/c", fmt.Sprintf("netstat -ano | findstr /r /c:\":%d \"", port))
		output, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		var pids []int
		for _, line := range strings.Split(string(output), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				if pid, err := strconv.Atoi(fields[len(fields)-1]); err == nil && pid > 0 {
					pids = append(pids, pid)
				}
			}
		}
		return pids, nil
	default:
		cmd := exec.Command("lsof", "-t", "-i", fmt.Sprintf(":%d", port))
		output, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		var pids []int
		for _, pidStr := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if pidStr == "" {
				continue
			}
			if pid, err := strconv.Atoi(pidStr); err == nil {
				pids = append(pids, pid)
			}
		}
		return pids, nil
	}
}

// isRoutaticProxyProcess checks if the PID belongs to routatic-proxy.
func (s *Server) isRoutaticProxyProcess(pid int) bool {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(output), "routatic-proxy")
	case "linux":
		cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			return false
		}
		return strings.Contains(string(cmdline), "routatic-proxy")
	default:
		output, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
		if err != nil {
			return false
		}
		name := strings.TrimSpace(string(output))
		return strings.Contains(name, "routatic-proxy")
	}
}

// killProcess terminates a process by PID. Returns true if successful.
func (s *Server) killProcess(pid int) bool {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
		return cmd.Run() == nil
	default:
		p, err := os.FindProcess(pid)
		if err != nil {
			return false
		}
		_ = p.Signal(os.Interrupt)
		time.Sleep(500 * time.Millisecond)
		_ = p.Kill()
		return true
	}
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
