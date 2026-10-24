package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/routatic/proxy/internal/config"
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

// requestedProvider treats an absent filter as all platforms, unlike routing's
// legacy empty-provider default. Validate before any query can widen the scope.
func requestedProvider(r *http.Request) (string, error) {
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	if provider == "" {
		return "", nil
	}
	provider = config.NormalizeProvider(provider)
	if !config.SupportedProvider(provider) {
		return "", errors.New("unsupported provider filter")
	}
	return provider, nil
}

// Summary returns high-level KPIs and breakdowns.
func (h *AnalyticsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	provider, err := requestedProvider(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
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
	window = window.ForProvider(provider)

	// One scan answers all four; the store folds them from a single pass and
	// caches it until the request table changes.
	snapshot, err := h.store.Summary(r.Context(), window)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := map[string]any{
		"provider":     provider,
		"summary":      snapshot.Summary,
		"models":       snapshot.Models,
		"providers":    snapshot.Providers,
		"scenarios":    snapshot.Scenarios,
		"generated_at": time.Now().Format(time.RFC3339),
	}
	if r.URL.Query().Get("compare") == "1" && !explicitRange {
		now := time.Now().UTC()
		// Trailing 60 seconds, deliberately not snapped: this figure is labelled
		// "last minute" and must keep meaning that. It is also the one range here
		// that never needs the cache - a one-minute window touches a handful of
		// rows through the start-time index.
		lastMinute, err := h.summaryBetween(r.Context(), now.Add(-time.Minute), now.Add(time.Second), provider)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Whole minutes, so this answer is cacheable across the three-second
		// poll. The extra margin reaches the end of the current minute, which
		// holds no rows yet and therefore changes nothing.
		now = now.Truncate(time.Minute)
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		today, err := h.summaryBetween(r.Context(), todayStart, now.Add(time.Minute), provider)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// The storage baseline still governs this wide window, so retained means
		// every trustworthy row rather than an arbitrary client-side date limit.
		retained, err := h.store.TokenSummary(r.Context(), h.store.Window(36500).ForProvider(provider))
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

func (h *AnalyticsHandler) summaryBetween(ctx context.Context, from, to time.Time, provider string) (*storage.TokenSummary, error) {
	window, err := h.store.WindowBetween(from, to)
	if err != nil {
		return nil, err
	}
	return h.store.TokenSummary(ctx, window.ForProvider(provider))
}

// TokenTrend returns daily token/request aggregates.
func (h *AnalyticsHandler) TokenTrend(w http.ResponseWriter, r *http.Request) {
	provider, err := requestedProvider(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
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
	trend, err := h.store.TokenTrend(r.Context(), window.ForProvider(provider), granularity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, map[string]any{
		"provider":    provider,
		"days":        days,
		"granularity": granularity,
		"trend":       trend,
	})
}
