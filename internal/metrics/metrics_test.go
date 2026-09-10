package metrics

import (
	"slices"
	"testing"
	"time"
)

func TestSnapshotLatencyPercentiles(t *testing.T) {
	descending := make([]time.Duration, 20)
	for i := range descending {
		descending[i] = time.Duration(20-i) * time.Millisecond
	}
	ascending := make([]time.Duration, 100)
	for i := range ascending {
		ascending[i] = time.Duration(i+1) * time.Millisecond
	}
	for _, tt := range []struct {
		name     string
		samples  []time.Duration
		p95, p99 time.Duration
	}{
		{name: "empty"},
		{name: "single", samples: []time.Duration{7 * time.Millisecond}, p95: 7 * time.Millisecond, p99: 7 * time.Millisecond},
		{name: "unsorted", samples: descending, p95: 19 * time.Millisecond, p99: 20 * time.Millisecond},
		{name: "exact nearest rank", samples: ascending, p95: 95 * time.Millisecond, p99: 99 * time.Millisecond},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			for _, sample := range tt.samples {
				m.RecordSuccess("test-model", sample)
			}
			snapshot := m.GetSnapshot()
			if got := snapshot.CalculateP95(); got != tt.p95 {
				t.Errorf("P95 = %v, want %v", got, tt.p95)
			}
			if got := snapshot.CalculateP99(); got != tt.p99 {
				t.Errorf("P99 = %v, want %v", got, tt.p99)
			}
			if !slices.Equal(snapshot.Latencies, tt.samples) {
				t.Error("percentile calculation mutated snapshot samples")
			}
		})
	}
}
