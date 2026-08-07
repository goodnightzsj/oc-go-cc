package storage

import (
	"encoding/json"
	"testing"
	"time"
)

// The dashboard reads st.p95_ms; this locks the wire field names and the
// millisecond unit so a future struct edit cannot silently break the KPI.
func TestModelLatencyStatsWireContract(t *testing.T) {
	s := ModelLatencyStats{
		Model: "m", Count: 2,
		Avg: 1500 * time.Millisecond,
		P50: 1000 * time.Millisecond,
		P90: 1800 * time.Millisecond,
		P95: 1900 * time.Millisecond,
		P99: 2000 * time.Millisecond,
		Min: 900 * time.Millisecond,
		Max: 2100 * time.Millisecond,
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]float64{
		"avg_ms": 1500, "p50_ms": 1000, "p90_ms": 1800,
		"p95_ms": 1900, "p99_ms": 2000, "min_ms": 900, "max_ms": 2100,
	} {
		v, ok := got[k]
		if !ok {
			t.Errorf("missing wire field %q; dashboard would render 0", k)
			continue
		}
		if v.(float64) != want {
			t.Errorf("%s = %v, want %v (must be whole ms, not ns)", k, v, want)
		}
	}
	if got["model"] != "m" {
		t.Errorf("model = %v, want lowercase json tag", got["model"])
	}
}

// p95 must sit between p90 and p99, and a tiny sample must still surface its
// slow tail rather than collapsing to the median.
func TestCalculateStatsP95(t *testing.T) {
	var samples []int64
	for i := 1; i <= 100; i++ {
		samples = append(samples, int64(i))
	}
	st := calculateStats("m", samples)
	if st.P95 != 95*time.Millisecond {
		t.Errorf("P95 = %v, want 95ms", st.P95)
	}
	if !(st.P90 <= st.P95 && st.P95 <= st.P99) {
		t.Errorf("percentiles out of order: p90=%v p95=%v p99=%v", st.P90, st.P95, st.P99)
	}
	tail := calculateStats("m", []int64{10, 500})
	if tail.P95 != 500*time.Millisecond {
		t.Errorf("2-sample P95 = %v, want 500ms (the slow tail)", tail.P95)
	}
}
