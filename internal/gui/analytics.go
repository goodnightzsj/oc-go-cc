package gui

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

// AnalyticsHandler serves analytics endpoints for the dashboard.
type AnalyticsHandler struct {
	store *storage.Analytics
}

// NewAnalyticsHandler creates a handler backed by the given database.
// It internally creates an Analytics store and a Latency store.
func NewAnalyticsHandler(db *storage.Database) *AnalyticsHandler {
	return &AnalyticsHandler{
		store: storage.NewAnalytics(db),
	}
}

func (h *AnalyticsHandler) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (h *AnalyticsHandler) getDays(r *http.Request) int {
	daysStr := r.URL.Query().Get("days")
	if daysStr == "" {
		return 30
	}
	d, err := strconv.Atoi(daysStr)
	if err != nil || d <= 0 {
		return 30
	}
	return d
}

func analyticsRange(r *http.Request) (time.Time, time.Time, bool, error) {
	fromValue := strings.TrimSpace(r.URL.Query().Get("from"))
	toValue := strings.TrimSpace(r.URL.Query().Get("to"))
	if fromValue == "" && toValue == "" {
		return time.Time{}, time.Time{}, false, nil
	}
	if fromValue == "" || toValue == "" {
		return time.Time{}, time.Time{}, false, errors.New("from and to are both required")
	}
	from, err := time.Parse(time.RFC3339, fromValue)
	if err != nil {
		return time.Time{}, time.Time{}, false, errors.New("invalid from time")
	}
	to, err := time.Parse(time.RFC3339, toValue)
	if err != nil {
		return time.Time{}, time.Time{}, false, errors.New("invalid to time")
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, false, errors.New("from must be before to")
	}
	if to.Sub(from) > 92*24*time.Hour {
		return time.Time{}, time.Time{}, false, errors.New("analytics range cannot exceed 92 days")
	}
	return from, to, true, nil
}

// Summary returns high-level KPIs and breakdowns.
func (h *AnalyticsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	days := h.getDays(r)
	from, to, explicitRange, rangeErr := analyticsRange(r)
	if rangeErr != nil {
		http.Error(w, rangeErr.Error(), http.StatusBadRequest)
		return
	}
	window, err := h.window(days, from, to, explicitRange)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	summary, err := h.store.TokenSummary(window)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	models, err := h.store.ModelBreakdown(window)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	providers, err := h.store.ProviderBreakdown(window)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scenarios, err := h.store.ScenarioBreakdown(window)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := map[string]any{
		"summary":      summary,
		"models":       models,
		"providers":    providers,
		"scenarios":    scenarios,
		"generated_at": time.Now().Format(time.RFC3339),
	}
	if r.URL.Query().Get("compare") == "1" && !explicitRange {
		now := time.Now()
		lastMinute, err := h.summaryBetween(now.Add(-time.Minute), now.Add(time.Second))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		today, err := h.summaryBetween(todayStart, now.Add(time.Second))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// The storage baseline still governs this wide window, so retained means
		// every trustworthy row rather than an arbitrary client-side date limit.
		retained, err := h.store.TokenSummary(h.store.Window(36500))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp["last_minute"] = lastMinute
		resp["today"] = today
		resp["retained"] = retained
	}
	h.writeJSON(w, resp)
}

// window resolves the request's analytics window once, so every panel in a
// response aggregates over exactly the same range. Range values have already
// been validated by analyticsRange; a leftover error here (a zero-valued but
// parseable timestamp) keeps its original 500 status.
func (h *AnalyticsHandler) window(days int, from, to time.Time, explicitRange bool) (storage.Window, error) {
	if explicitRange {
		return h.store.WindowBetween(from, to)
	}
	return h.store.Window(days), nil
}

func (h *AnalyticsHandler) summaryBetween(from, to time.Time) (*storage.TokenSummary, error) {
	window, err := h.store.WindowBetween(from, to)
	if err != nil {
		return nil, err
	}
	return h.store.TokenSummary(window)
}

// TokenTrend returns daily token/request aggregates.
func (h *AnalyticsHandler) TokenTrend(w http.ResponseWriter, r *http.Request) {
	days := h.getDays(r)
	from, to, explicitRange, rangeErr := analyticsRange(r)
	if rangeErr != nil {
		http.Error(w, rangeErr.Error(), http.StatusBadRequest)
		return
	}
	granularity := strings.TrimSpace(r.URL.Query().Get("granularity"))
	if granularity == "" {
		granularity = "day"
	}
	if granularity != "day" && granularity != "hour" {
		http.Error(w, "granularity must be day or hour", http.StatusBadRequest)
		return
	}
	window, err := h.window(days, from, to, explicitRange)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	trend, err := h.store.TokenTrend(window, granularity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, map[string]any{
		"days":        days,
		"granularity": granularity,
		"trend":       trend,
	})
}
