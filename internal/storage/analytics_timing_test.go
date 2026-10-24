package storage

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func assertContentAggregate(t *testing.T, value any, samples int64, want *float64) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["content_samples"] != float64(samples) {
		t.Errorf("content_samples = %v, want %d", fields["content_samples"], samples)
	}
	rate, exists := fields["content_tokens_per_second"]
	if want == nil {
		if exists {
			t.Errorf("unknown content rate = %v, want omitted", rate)
		}
	} else if got, ok := rate.(float64); !ok || math.Abs(got-*want) > 1e-9 {
		t.Errorf("content rate = %v, want %v", rate, *want)
	}
}

func TestContentAggregateWeightsSamplesAndTracksWindowAndWrites(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	zero, one, hundredOne := int64(0), int64(1000), int64(101000)
	first := history.RequestRecord{ID: "fast", Model: "alpha", Provider: "p", StartTime: at,
		Streaming: true, Success: true, OutputTokens: 1000, ReasoningTokens: 200,
		Duration: 2 * time.Second, FirstContentMs: &zero, MessageEndMs: &one}
	insertCostRecord(t, db, first)
	second := first
	second.ID, second.OutputTokens, second.Duration = "slow", 10, 110*time.Second
	second.StartTime, second.FirstContentMs, second.MessageEndMs = at.Add(24*time.Hour), &one, &hundredOne
	insertCostRecord(t, db, second)
	other := first
	other.ID, other.Provider, other.OutputTokens = "other", "q", 50
	insertCostRecord(t, db, other)
	a := NewAnalytics(db)
	window, err := a.WindowBetween(at, at.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	window = window.ForProvider("p")
	snap, err := a.Summary(t.Context(), window)
	if err != nil || len(snap.Models) != 1 || len(snap.Providers) != 1 {
		t.Fatalf("snapshot = %+v, err = %v", snap, err)
	}
	want := 1010.0 / 101.0
	assertContentAggregate(t, snap.Models[0], 2, &want)
	assertContentAggregate(t, snap.Providers[0], 2, &want)
	if math.Abs(snap.Models[0].TokensPerSecond-1010.0/112.0) > 1e-9 {
		t.Fatal("execution throughput changed")
	}
	cached, err := a.Summary(t.Context(), window)
	if err != nil || cached != snap {
		t.Fatalf("snapshot cache not reused: %v", err)
	}
	narrow, err := a.WindowBetween(at, at.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	narrowSnap, err := a.Summary(t.Context(), narrow.ForProvider("p"))
	if err != nil {
		t.Fatal(err)
	}
	want = 1000
	assertContentAggregate(t, narrowSnap.Models[0], 1, &want)
	first.ID, first.OutputTokens = "new", 100
	insertCostRecord(t, db, first)
	after, err := a.Summary(t.Context(), window)
	if err != nil || after == snap {
		t.Fatalf("snapshot not refreshed: %v", err)
	}
	want = 1110.0 / 102.0
	assertContentAggregate(t, after.Models[0], 3, &want)
	assertContentAggregate(t, after.Providers[0], 3, &want)
}

func TestContentAggregatePopulationMatchesHistoryRate(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	zero, ten, negative, beyond := int64(0), int64(10000), int64(-1), int64(20000)
	wantByModel := map[string]*float64{}
	for name, mutate := range map[string]func(*history.RequestRecord){
		"valid_zero":  func(r *history.RequestRecord) {},
		"legacy":      func(r *history.RequestRecord) { r.FirstContentMs, r.MessageEndMs = nil, nil },
		"nonstream":   func(r *history.RequestRecord) { r.Streaming = false },
		"failure":     func(r *history.RequestRecord) { r.Success = false },
		"unobserved":  func(r *history.RequestRecord) { r.UsageUnobserved, r.DetailsKnown = true, false },
		"zero_output": func(r *history.RequestRecord) { r.OutputTokens = 0 },
		"missing_end": func(r *history.RequestRecord) { r.MessageEndMs = nil },
		"negative":    func(r *history.RequestRecord) { r.FirstContentMs = &negative },
		"equal":       func(r *history.RequestRecord) { r.FirstContentMs = &ten },
		"reverse":     func(r *history.RequestRecord) { r.FirstContentMs = &beyond },
		"beyond_end":  func(r *history.RequestRecord) { r.MessageEndMs = &beyond },
	} {
		r := history.RequestRecord{ID: name, Model: name, Provider: "p", StartTime: at,
			Streaming: true, Success: true, DetailsKnown: true, OutputTokens: 600, ReasoningTokens: 100,
			Duration: 16 * time.Second, FirstContentMs: &zero, MessageEndMs: &ten}
		mutate(&r)
		insertCostRecord(t, db, r)
		wantByModel[name] = r.ContentTokensPerSecond()
	}
	a := NewAnalytics(db)
	window, err := a.WindowBetween(at, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := a.Summary(t.Context(), window)
	if err != nil || len(snap.Models) != len(wantByModel) {
		t.Fatalf("snapshot = %+v err = %v", snap, err)
	}
	for _, row := range snap.Models {
		t.Run(row.Model, func(t *testing.T) {
			var samples int64
			if row.Model == "valid_zero" {
				samples = 1
			}
			assertContentAggregate(t, row, samples, wantByModel[row.Model])
		})
	}
	want := 60.0
	assertContentAggregate(t, snap.Providers[0], 1, &want)
}

func BenchmarkContentSummaryLoad(b *testing.B) {
	db := summaryLoadDB(b, 65000)
	if _, err := db.DB().Exec(`UPDATE requests SET streaming=1, first_event_ms=100,
		first_content_ms=200, message_end_ms=duration_ms-100`); err != nil {
		b.Fatal(err)
	}
	query := summaryLoadQueries(b.Context(), db)["analytics"]
	for _, mode := range []string{"cold", "hot"} {
		b.Run(mode, func(b *testing.B) {
			if _, err := query(); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for range b.N {
				if mode == "cold" {
					b.StopTimer()
					if _, err := db.DB().Exec("UPDATE history_revision SET revision=revision+1 WHERE id=1"); err != nil {
						b.Fatal(err)
					}
					b.StartTimer()
				}
				if _, err := query(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
