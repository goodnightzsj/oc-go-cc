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
	db       *Database
	provider string
}

// NewLatency creates a new Latency repository backed by the given database.
func NewLatency(db *Database) *Latency {
	return &Latency{db: db}
}

// ForProvider leaves the shared repository unchanged while scoping both samples
// and outcome counts to the same provider. Empty preserves the all-provider view.
func (l *Latency) ForProvider(provider string) *Latency {
	return &Latency{db: l.db, provider: provider}
}

// ModelLatencyStats holds latency percentiles and counts for one provider/model.
//
// The JSON tags matter: /api/analytics/latency serialises this type straight to
// the dashboard, so the field names below are the wire contract. Durations are
// exposed as whole milliseconds (via MarshalJSON) rather than Go's native
// nanoseconds, because that is the unit the UI labels and renders.
type ModelLatencyStats struct {
	Provider string        `json:"provider"`
	Model    string        `json:"model"`
	Count    int64         `json:"count"`
	Avg      time.Duration `json:"-"`
	P50      time.Duration `json:"-"`
	P90      time.Duration `json:"-"`
	P95      time.Duration `json:"-"`
	P99      time.Duration `json:"-"`
	Min      time.Duration `json:"-"`
	Max      time.Duration `json:"-"`
}

// MarshalJSON emits millisecond fields so the dashboard reads plain numbers in
// the unit it displays, instead of raw nanosecond durations.
func (s ModelLatencyStats) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Count    int64  `json:"count"`
		AvgMs    int64  `json:"avg_ms"`
		P50Ms    int64  `json:"p50_ms"`
		P90Ms    int64  `json:"p90_ms"`
		P95Ms    int64  `json:"p95_ms"`
		P99Ms    int64  `json:"p99_ms"`
		MinMs    int64  `json:"min_ms"`
		MaxMs    int64  `json:"max_ms"`
	}{
		Provider: s.Provider,
		Model:    s.Model,
		Count:    s.Count,
		AvgMs:    s.Avg.Milliseconds(),
		P50Ms:    s.P50.Milliseconds(),
		P90Ms:    s.P90.Milliseconds(),
		P95Ms:    s.P95.Milliseconds(),
		P99Ms:    s.P99.Milliseconds(),
		MinMs:    s.Min.Milliseconds(),
		MaxMs:    s.Max.Milliseconds(),
	})
}

// maxSamplesPerModel bounds how many request rows GetStats pulls per provider/model.
// Percentiles over the most recent 20k samples are representative, and this
// keeps a long-running proxy from loading millions of rows into memory.
const maxSamplesPerModel = 20000

// GetStats returns latency statistics for all models with samples recorded after the given time.
func (l *Latency) GetStats(since time.Time) ([]ModelLatencyStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Cap samples per provider/model so a chatty platform cannot starve another
	// platform's samples for the same model. Each Count is a bounded sample count,
	// not the total number of requests used to calculate success rates.
	// duration_ms on requests is the same measurement the dedicated
	// latency_samples table used to duplicate. Synthetic rows imported by
	// `costs sync-requests` carry duration_ms=0 and details_known=0, so they are
	// excluded rather than being counted as instant responses.
	query := `
		SELECT provider, model, duration_ms
		FROM (
			SELECT COALESCE(provider, '') AS provider, model, duration_ms,
			       ROW_NUMBER() OVER (PARTITION BY COALESCE(provider, ''), model ORDER BY julianday(start_time) DESC) AS rn
			FROM requests
				WHERE julianday(start_time) >= julianday(?)
				  AND (? = '' OR provider = ?)
			  AND duration_ms > 0
			  AND details_known = 1
		)
		WHERE rn <= ?
		ORDER BY provider, model
	`

	rows, err := l.db.DB().QueryContext(ctx, query, since.UTC().Format(time.RFC3339Nano), l.provider, l.provider, maxSamplesPerModel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type modelKey struct{ provider, model string }
	samplesByModel := make(map[modelKey][]int64)
	for rows.Next() {
		var key modelKey
		var latencyMs int64
		if err := rows.Scan(&key.provider, &key.model, &latencyMs); err != nil {
			return nil, err
		}
		samplesByModel[key] = append(samplesByModel[key], latencyMs)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	var stats []ModelLatencyStats
	for key, samples := range samplesByModel {
		stat := calculateStats(key.model, samples)
		stat.Provider = key.provider
		stats = append(stats, stat)
	}

	return stats, nil
}

// GetSuccessCounts returns known request outcomes since the given time, keyed
// by provider + "/" + model. Empty legacy providers retain the leading slash.
// Imported billing rows have unknown outcomes and do not enter either count.
func (l *Latency) GetSuccessCounts(since time.Time) (map[string]int64, map[string]int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT COALESCE(provider, ''), model, success, COUNT(*)
		FROM requests
			WHERE julianday(start_time) >= julianday(?)
			  AND (? = '' OR provider = ?)
		  AND details_known = 1
		  AND success IN (0, 1)
		GROUP BY COALESCE(provider, ''), model, success
	`

	rows, err := l.db.DB().QueryContext(ctx, query, since.UTC().Format(time.RFC3339Nano), l.provider, l.provider)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	successCounts := make(map[string]int64)
	failureCounts := make(map[string]int64)

	for rows.Next() {
		var provider, model string
		var success, count int64
		if err := rows.Scan(&provider, &model, &success, &count); err != nil {
			return nil, nil, err
		}
		key := provider + "/" + model
		if success == 1 {
			successCounts[key] = count
		} else {
			failureCounts[key] = count
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

	p50Idx := pctIdx(0.50)
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
	case "90d":
		return time.Now().Add(-90 * 24 * time.Hour)
	default:
		return time.Time{}
	}
}
