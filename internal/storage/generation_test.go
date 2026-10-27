package storage

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/metrics"
)

func TestGenerationStorageRoundTrip(t *testing.T) {
	db := newCostTestDB(t)
	repo := NewRequests(db)
	for _, sample := range []*metrics.GenerationSample{nil, {FirstOutputMs: 0, DurationMs: 100, OutputTokens: 0}, {FirstOutputMs: 1000, DurationMs: 99, OutputTokens: 20}, {FirstOutputMs: 1000, DurationMs: 2000, OutputTokens: 200}} {
		rec := history.RequestRecord{ID: "sample", Model: "synthetic", StartTime: time.Now(), Duration: 10 * time.Second, Streaming: true, Success: true, OutputTokens: 999, GenerationSample: sample}
		if err := repo.Insert(rec); err != nil {
			t.Fatal(err)
		}
		rows, _, err := repo.Query(t.Context(), RequestQuery{})
		if err != nil || len(rows) != 1 {
			t.Fatal(rows, err)
		}
		if !reflect.DeepEqual(rows[0].GenerationSample, sample) || rows[0].OutputTokens != 999 {
			t.Fatalf("sample/billing changed: %+v", rows[0])
		}
	}
}

func TestGenerationMigrationAndOldWriter(t *testing.T) {
	db := newCostTestDB(t)
	for _, column := range []string{"generation_first_output_ms", "generation_duration_ms", "generation_output_tokens"} {
		if _, err := db.DB().Exec("ALTER TABLE requests DROP COLUMN " + column); err != nil {
			t.Fatal(err)
		}
	}
	oldInsert := `INSERT INTO requests (id,model,start_time,duration_ms,input_tokens,output_tokens,first_content_ms,message_end_ms) VALUES (?,'synthetic','2026-10-25T00:00:00Z',1000,10,20,100,900)`
	if _, err := db.DB().Exec(oldInsert, "old"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := db.migrateColumns(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.DB().Exec(oldInsert, "old-writer"); err != nil {
		t.Fatal(err)
	}
	var count, output int
	if err := db.DB().QueryRow(`SELECT COUNT(*),SUM(output_tokens) FROM requests WHERE generation_first_output_ms IS NULL AND generation_duration_ms IS NULL AND generation_output_tokens IS NULL AND first_content_ms=100 AND message_end_ms=900`).Scan(&count, &output); err != nil || count != 2 || output != 40 {
		t.Fatalf("legacy data altered: %d %d %v", count, output, err)
	}
}

func TestGenerationRejectsInvalidSamplesWithoutReplacingRecord(t *testing.T) {
	db := newCostTestDB(t)
	repo := NewRequests(db)
	base := history.RequestRecord{ID: "sample", Model: "synthetic", StartTime: time.Now(), Duration: time.Second, Success: true, Streaming: true, OutputTokens: 10}
	if err := repo.Insert(base); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*history.RequestRecord){
		func(r *history.RequestRecord) { r.GenerationSample.FirstOutputMs = -1 },
		func(r *history.RequestRecord) { r.GenerationSample.DurationMs = -1 },
		func(r *history.RequestRecord) { r.GenerationSample.DurationMs = 1001 },
		func(r *history.RequestRecord) { r.GenerationSample.FirstOutputMs = 950 },
		func(r *history.RequestRecord) { r.GenerationSample.OutputTokens = -1 },
		func(r *history.RequestRecord) { r.GenerationSample.OutputTokens = 1 << 53 },
		func(r *history.RequestRecord) { r.Streaming = false },
		func(r *history.RequestRecord) { r.Success = false },
		func(r *history.RequestRecord) { r.UsageUnobserved = true },
	} {
		rec := base
		rec.OutputTokens = 999
		rec.GenerationSample = &metrics.GenerationSample{DurationMs: 100, OutputTokens: 10}
		mutate(&rec)
		if err := repo.Insert(rec); err == nil {
			t.Fatalf("accepted invalid sample: %+v", rec)
		}
	}
	rows, _, err := repo.Query(t.Context(), RequestQuery{})
	if err != nil || len(rows) != 1 || rows[0].OutputTokens != 10 || rows[0].GenerationSample != nil {
		t.Fatal("invalid input replaced record", rows, err)
	}
}

func assertGenerationAggregate(t *testing.T, value any, samples int64, want *float64) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["generation_samples"] != float64(samples) {
		t.Fatalf("sample count: %s", raw)
	}
	rate, exists := fields["generation_tokens_per_second"]
	if want == nil {
		if exists {
			t.Fatalf("unknown rate not omitted: %s", raw)
		}
	} else if got, ok := rate.(float64); !ok || math.Abs(got-*want) > 1e-9 {
		t.Fatalf("rate=%v want=%v", rate, *want)
	}
}

func TestGenerationAggregateWeightsSameSourceAndPreservesCache(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	for _, rec := range []history.RequestRecord{
		{ID: "fast", Provider: "p", GenerationSample: &metrics.GenerationSample{FirstOutputMs: 1000, DurationMs: 1000, OutputTokens: 1000}},
		{ID: "slow", Provider: "p", GenerationSample: &metrics.GenerationSample{FirstOutputMs: 1000, DurationMs: 10000, OutputTokens: 10}},
		{ID: "zero", Provider: "p", GenerationSample: &metrics.GenerationSample{DurationMs: 1000, OutputTokens: 0}},
		{ID: "short", Provider: "p", GenerationSample: &metrics.GenerationSample{DurationMs: 99, OutputTokens: 9999}},
		{ID: "legacy", Provider: "p"},
		{ID: "other", Provider: "q", GenerationSample: &metrics.GenerationSample{DurationMs: 1000, OutputTokens: 555}},
	} {
		rec.Model, rec.StartTime, rec.Duration, rec.Streaming, rec.Success, rec.OutputTokens = "model", at, 20*time.Second, true, true, 2000
		insertCostRecord(t, db, rec)
	}
	a := NewAnalytics(db)
	window, err := a.WindowBetween(at, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	window = window.ForProvider("p")
	snap, err := a.Summary(t.Context(), window)
	if err != nil || len(snap.Models) != 1 || len(snap.Providers) != 1 {
		t.Fatal(snap, err)
	}
	want := 1010.0 / 12.0
	assertGenerationAggregate(t, snap.Models[0], 3, &want)
	assertGenerationAggregate(t, snap.Providers[0], 3, &want)
	if snap.Models[0].TokensPerSecond != 100 {
		t.Fatal("legacy average changed")
	}
	cached, err := a.Summary(t.Context(), window)
	if err != nil || cached != snap {
		t.Fatal("cache not reused", err)
	}
	insertCostRecord(t, db, history.RequestRecord{ID: "new", Provider: "p", Model: "model", StartTime: at, Duration: time.Second, Streaming: true, Success: true, GenerationSample: &metrics.GenerationSample{DurationMs: 1000, OutputTokens: 30}})
	after, err := a.Summary(t.Context(), window)
	if err != nil || after == snap {
		t.Fatal("cache not invalidated", err)
	}
	want = 1040.0 / 13.0
	assertGenerationAggregate(t, after.Models[0], 4, &want)
	assertGenerationAggregate(t, after.Providers[0], 4, &want)
}

func TestGenerationPopulationAndSortMatchHistory(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	for _, rec := range []history.RequestRecord{
		{ID: "fast", GenerationSample: &metrics.GenerationSample{DurationMs: 100, OutputTokens: 10}},
		{ID: "slow", GenerationSample: &metrics.GenerationSample{DurationMs: 1000, OutputTokens: 10}},
		{ID: "zero", GenerationSample: &metrics.GenerationSample{DurationMs: 1000, OutputTokens: 0}},
		{ID: "short", GenerationSample: &metrics.GenerationSample{DurationMs: 99, OutputTokens: 1000}},
		{ID: "legacy"},
	} {
		rec.Model, rec.Provider, rec.StartTime, rec.Duration, rec.Streaming, rec.Success = rec.ID, "p", at, 10*time.Second, true, true
		insertCostRecord(t, db, rec)
	}
	for _, order := range []string{"asc", "desc"} {
		rows, _, err := NewRequests(db).Query(t.Context(), RequestQuery{SortBy: "generation_tokens_per_second", SortOrder: order})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"zero", "slow", "fast", "legacy", "short"}
		if order == "desc" {
			want = []string{"fast", "slow", "zero", "legacy", "short"}
		}
		for i, r := range rows {
			if r.ID != want[i] {
				t.Fatalf("%s sort row %d=%s want=%s", order, i, r.ID, want[i])
			}
		}
	}
	a := NewAnalytics(db)
	window, err := a.WindowBetween(at, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := a.Summary(t.Context(), window)
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := NewRequests(db).Query(t.Context(), RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		for _, model := range snap.Models {
			if model.Model == row.Model {
				var samples int64
				if row.GenerationTokensPerSecond() != nil {
					samples = 1
				}
				assertGenerationAggregate(t, model, samples, row.GenerationTokensPerSecond())
			}
		}
	}
}

func BenchmarkGenerationSummaryLoad(b *testing.B) {
	db := summaryLoadDB(b, 65000)
	if _, err := db.DB().Exec(`UPDATE requests SET streaming=1, generation_first_output_ms=100, generation_duration_ms=MAX(0,duration_ms-200), generation_output_tokens=output_tokens`); err != nil {
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
