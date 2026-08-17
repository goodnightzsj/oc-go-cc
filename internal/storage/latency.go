package storage

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"time"
)

// Latency provides methods for recording and querying latency samples.
type Latency struct {
	db *Database
}

// NewLatency creates a new Latency repository backed by the given database.
func NewLatency(db *Database) *Latency {
	return &Latency{db: db}
}

// ModelLatencyStats holds latency percentiles and counts for a single model.
//
// The JSON tags matter: /api/analytics/latency serialises this type straight to
// the dashboard, so the field names below are the wire contract. Durations are
// exposed as whole milliseconds (via MarshalJSON) rather than Go's native
// nanoseconds, because that is the unit the UI labels and renders.
type ModelLatencyStats struct {
	Model string        `json:"model"`
	Count int64         `json:"count"`
	Avg   time.Duration `json:"-"`
	P50   time.Duration `json:"-"`
	P90   time.Duration `json:"-"`
	P95   time.Duration `json:"-"`
	P99   time.Duration `json:"-"`
	Min   time.Duration `json:"-"`
	Max   time.Duration `json:"-"`
}

// MarshalJSON emits millisecond fields so the dashboard reads plain numbers in
// the unit it displays, instead of raw nanosecond durations.
func (s ModelLatencyStats) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Model string `json:"model"`
		Count int64  `json:"count"`
		AvgMs int64  `json:"avg_ms"`
		P50Ms int64  `json:"p50_ms"`
		P90Ms int64  `json:"p90_ms"`
		P95Ms int64  `json:"p95_ms"`
		P99Ms int64  `json:"p99_ms"`
		MinMs int64  `json:"min_ms"`
		MaxMs int64  `json:"max_ms"`
	}{
		Model: s.Model,
		Count: s.Count,
		AvgMs: s.Avg.Milliseconds(),
		P50Ms: s.P50.Milliseconds(),
		P90Ms: s.P90.Milliseconds(),
		P95Ms: s.P95.Milliseconds(),
		P99Ms: s.P99.Milliseconds(),
		MinMs: s.Min.Milliseconds(),
		MaxMs: s.Max.Milliseconds(),
	})
}

// maxSamplesPerModel bounds how many request rows GetStats pulls per model.
// Percentiles over the most recent 20k samples are representative, and this
// keeps a long-running proxy from loading millions of rows into memory.
const maxSamplesPerModel = 20000

// GetStats returns latency statistics for all models with samples recorded after the given time.
func (l *Latency) GetStats(since time.Time) ([]ModelLatencyStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Cap the samples per model rather than overall: percentiles need a model's
	// own distribution, so a global LIMIT would let one chatty model starve the
	// others. Keeping the most recent maxSamplesPerModel rows per model bounds
	// memory while leaving each model's recent distribution intact.
	// duration_ms on requests is the same measurement the dedicated
	// latency_samples table used to duplicate. Synthetic rows imported by
	// `costs sync-requests` carry duration_ms=0 and details_known=0, so they are
	// excluded rather than being counted as instant responses.
	query := `
		SELECT model, duration_ms
		FROM (
			SELECT model, duration_ms,
			       ROW_NUMBER() OVER (PARTITION BY model ORDER BY start_time DESC) AS rn
			FROM requests
			WHERE start_time >= ?
			  AND duration_ms > 0
			  AND details_known = 1
		)
		WHERE rn <= ?
		ORDER BY model
	`

	rows, err := l.db.DB().QueryContext(ctx, query, since.Format(time.RFC3339Nano), maxSamplesPerModel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	samplesByModel := make(map[string][]int64)
	for rows.Next() {
		var model string
		var latencyMs int64
		if err := rows.Scan(&model, &latencyMs); err != nil {
			return nil, err
		}
		samplesByModel[model] = append(samplesByModel[model], latencyMs)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	var stats []ModelLatencyStats
	for model, samples := range samplesByModel {
		stats = append(stats, calculateStats(model, samples))
	}

	return stats, nil
}

// GetSuccessCounts returns the count of successful and failed requests per model since the given time.
func (l *Latency) GetSuccessCounts(since time.Time) (map[string]int64, map[string]int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT model, success, COUNT(*)
		FROM requests
		WHERE start_time >= ?
		GROUP BY model, success
	`

	rows, err := l.db.DB().QueryContext(ctx, query, since.Format(time.RFC3339Nano))
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	successCounts := make(map[string]int64)
	failureCounts := make(map[string]int64)

	for rows.Next() {
		var model string
		var success, count int64
		if err := rows.Scan(&model, &success, &count); err != nil {
			return nil, nil, err
		}
		if success == 1 {
			successCounts[model] = count
		} else {
			failureCounts[model] = count
		}
	}

	return successCounts, failureCounts, rows.Err()
}

func calculateStats(model string, samples []int64) ModelLatencyStats {
	if len(samples) == 0 {
		return ModelLatencyStats{Model: model}
	}

	sorted := slices.Clone(samples)
	slices.Sort(sorted)

	var sum int64
	for _, ms := range sorted {
		sum += ms
	}

	count := len(sorted)
	avg := sum / int64(count)

	pctIdx := func(fraction float64) int {
		return min(max(int(math.Ceil(float64(count)*fraction))-1, 0), count-1)
	}

	// p50 keeps its original truncating form; the upper percentiles round up so
	// a small sample set still reports its slow tail instead of the median.
	p50Idx := min(max(int(float64(count)*0.50)-1, 0), count-1)
	p90Idx := pctIdx(0.90)
	p95Idx := pctIdx(0.95)
	p99Idx := pctIdx(0.99)

	return ModelLatencyStats{
		Model: model,
		Count: int64(count),
		Avg:   time.Duration(avg) * time.Millisecond,
		P50:   time.Duration(sorted[p50Idx]) * time.Millisecond,
		P90:   time.Duration(sorted[p90Idx]) * time.Millisecond,
		P95:   time.Duration(sorted[p95Idx]) * time.Millisecond,
		P99:   time.Duration(sorted[p99Idx]) * time.Millisecond,
		Min:   time.Duration(sorted[0]) * time.Millisecond,
		Max:   time.Duration(sorted[count-1]) * time.Millisecond,
	}
}

// ParseTimeRange parses a range parameter ("1h", "24h", "7d", "30d") and returns
// the corresponding point in the past.
func ParseTimeRange(rangeParam string) time.Time {
	switch rangeParam {
	case "1h":
		return time.Now().Add(-1 * time.Hour)
	case "24h":
		return time.Now().Add(-24 * time.Hour)
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		return time.Now().Add(-30 * 24 * time.Hour)
	default:
		return time.Time{}
	}
}
