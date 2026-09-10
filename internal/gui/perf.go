package gui

import (
	"net/http"
	"strings"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

type modelPerf struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Count    int64  `json:"count"`
	Success  int64  `json:"success"`
	Failed   int64  `json:"failed"`
	AvgMs    int64  `json:"avg_ms"`
	P50Ms    int64  `json:"p50_ms"`
	P90Ms    int64  `json:"p90_ms"`
	P99Ms    int64  `json:"p99_ms"`
	MinMs    int64  `json:"min_ms"`
	MaxMs    int64  `json:"max_ms"`
}

func modelPerfFromFields(model string, count int64, avg, p50, p90, p99, min, max time.Duration) modelPerf {
	return modelPerf{
		Model: model,
		Count: count,
		AvgMs: avg.Milliseconds(),
		P50Ms: p50.Milliseconds(),
		P90Ms: p90.Milliseconds(),
		P99Ms: p99.Milliseconds(),
		MinMs: min.Milliseconds(),
		MaxMs: max.Milliseconds(),
	}
}

func (s *Server) handlePerformance(w http.ResponseWriter, r *http.Request) {
	provider, err := requestedProvider(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if provider != "" && s.storage == nil {
		http.Error(w, "platform performance requires persistent storage", http.StatusServiceUnavailable)
		return
	}
	rangeParam := r.URL.Query().Get("range")
	since := storage.ParseTimeRange(rangeParam)

	result := make(map[string]modelPerf)

	if s.storage != nil {
		latency := storage.NewLatency(s.storage).ForProvider(provider)

		modelStats, err := latency.GetStats(since)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, stat := range modelStats {
			perf := modelPerfFromFields(stat.Model, stat.Count, stat.Avg, stat.P50, stat.P90, stat.P99, stat.Min, stat.Max)
			perf.Provider = stat.Provider
			result[stat.Provider+"/"+stat.Model] = perf
		}

		successCounts, failureCounts, err := latency.GetSuccessCounts(since)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for key, count := range successCounts {
			perf := result[key]
			perf.Provider, perf.Model, _ = strings.Cut(key, "/")
			perf.Success = count
			result[key] = perf
		}
		for key, count := range failureCounts {
			perf := result[key]
			perf.Provider, perf.Model, _ = strings.Cut(key, "/")
			perf.Failed = count
			result[key] = perf
		}
	} else if s.met != nil {
		snap := s.met.GetSnapshot()
		modelStats := s.met.GetModelLatencyStats()

		for _, stat := range modelStats {
			result[stat.Model] = modelPerfFromFields(stat.Model, stat.Count, stat.Avg, stat.P50, stat.P90, stat.P99, stat.Min, stat.Max)
		}

		for model, count := range snap.ModelCounts {
			if perf, exists := result[model]; exists {
				perf.Success = snap.ModelSuccess[model]
				perf.Failed = snap.ModelFailed[model]
				result[model] = perf
			} else {
				result[model] = modelPerf{
					Model:   model,
					Count:   count,
					Success: snap.ModelSuccess[model],
					Failed:  snap.ModelFailed[model],
				}
			}
		}
	}

	var output []modelPerf
	for _, perf := range result {
		output = append(output, perf)
	}

	writeJSON(w, output)
}

func (s *Server) handlePerformanceAggregate(w http.ResponseWriter, r *http.Request) {
	provider, err := requestedProvider(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if provider != "" && s.storage == nil {
		http.Error(w, "platform performance requires persistent storage", http.StatusServiceUnavailable)
		return
	}
	rangeParam := r.URL.Query().Get("range")
	since := storage.ParseTimeRange(rangeParam)

	type aggregate struct {
		TotalRequests int64 `json:"total_requests"`
		TotalSuccess  int64 `json:"total_success"`
		TotalFailed   int64 `json:"total_failed"`
		AvgLatencyMs  int64 `json:"avg_latency_ms"`
	}

	agg := aggregate{}

	if s.storage != nil {
		latency := storage.NewLatency(s.storage).ForProvider(provider)
		latencyStats, err := latency.GetStats(since)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(latencyStats) > 0 {
			var totalCount int64
			var totalLatency time.Duration
			for _, stat := range latencyStats {
				totalCount += stat.Count
				totalLatency += stat.Avg * time.Duration(stat.Count)
			}
			if totalCount > 0 {
				agg.AvgLatencyMs = (totalLatency / time.Duration(totalCount)).Milliseconds()
			}
		}

		successCounts, failureCounts, err := latency.GetSuccessCounts(since)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, s := range successCounts {
			agg.TotalSuccess += s
		}
		for _, f := range failureCounts {
			agg.TotalFailed += f
		}
		agg.TotalRequests = agg.TotalSuccess + agg.TotalFailed
	} else if s.met != nil {
		snap := s.met.GetSnapshot()
		agg.TotalRequests = snap.RequestsReceived
		agg.TotalSuccess = snap.RequestsSuccess
		agg.TotalFailed = snap.RequestsFailed
		if len(snap.Latencies) > 0 {
			var sum time.Duration
			for _, lat := range snap.Latencies {
				sum += lat
			}
			agg.AvgLatencyMs = (sum / time.Duration(len(snap.Latencies))).Milliseconds()
		}
	}

	writeJSON(w, agg)
}
