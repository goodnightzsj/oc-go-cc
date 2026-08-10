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
	store   *storage.Analytics
	latency *storage.Latency
}

// NewAnalyticsHandler creates a handler backed by the given database.
// It internally creates an Analytics store and a Latency store.
func NewAnalyticsHandler(db *storage.Database) *AnalyticsHandler {
	return &AnalyticsHandler{
		store:   storage.NewAnalytics(db),
		latency: storage.NewLatency(db),
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

	var summary *storage.TokenSummary
	var err error
	if explicitRange {
		summary, err = h.store.GetTokenSummaryBetween(from, to)
	} else {
		summary, err = h.store.GetTokenSummary(days)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var models []storage.ModelBreakdown
	if explicitRange {
		models, err = h.store.GetModelBreakdownBetween(from, to)
	} else {
		models, err = h.store.GetModelBreakdown(days)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var providers []storage.ProviderBreakdown
	if explicitRange {
		providers, err = h.store.GetProviderBreakdownBetween(from, to)
	} else {
		providers, err = h.store.GetProviderBreakdown(days)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var scenarios []storage.ScenarioBreakdown
	if explicitRange {
		scenarios, err = h.store.GetScenarioBreakdownBetween(from, to)
	} else {
		scenarios, err = h.store.GetScenarioBreakdown(days)
	}
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
	h.writeJSON(w, resp)
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
	var trend []storage.DailyTokenPoint
	var err error
	if explicitRange {
		trend, err = h.store.GetTokenTrendBetween(from, to, granularity)
	} else if granularity == "hour" {
		end := time.Now().Add(time.Second)
		trend, err = h.store.GetTokenTrendBetween(end.AddDate(0, 0, -days), end, granularity)
	} else {
		trend, err = h.store.GetDailyTokenTrend(days)
	}
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

// LatencyStats returns latency stats per model.
func (h *AnalyticsHandler) LatencyStats(w http.ResponseWriter, r *http.Request) {
	days := h.getDays(r)
	since := time.Now().AddDate(0, 0, -days)
	stats, err := h.latency.GetStats(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, map[string]any{
		"days":  days,
		"stats": stats,
	})
}
