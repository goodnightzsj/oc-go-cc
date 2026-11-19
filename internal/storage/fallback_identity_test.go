package storage

import (
	"database/sql"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestCandidatePositionMigrationKeepsLegacyFallbackIdentity(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	if _, err := db.DB().Exec(`ALTER TABLE requests DROP COLUMN candidate_position`); err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []int{1, 2} {
		if _, err := db.DB().Exec(`INSERT INTO requests
			(id, model, provider, start_time, duration_ms, input_tokens, output_tokens, attempt, success, outcome_known)
			VALUES (?, 'model', 'synthetic', ?, 1000, 0, 0, ?, 1, 1)`, attempt, at.Format(time.RFC3339Nano), attempt); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := db.migrateColumns(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	var candidate sql.NullInt64
	if err := db.DB().QueryRow(`SELECT candidate_position FROM requests WHERE id = '2'`).Scan(&candidate); err != nil || candidate.Valid {
		t.Fatalf("legacy candidate must stay NULL: %+v err=%v", candidate, err)
	}
	rows, total, err := NewRequests(db).Query(t.Context(), RequestQuery{})
	if err != nil || total != 2 || rows[0].CandidatePosition != 0 || rows[1].CandidatePosition != 0 {
		t.Fatalf("legacy readback: rows=%+v total=%d err=%v", rows, total, err)
	}
	a := NewAnalytics(db)
	window, err := a.WindowBetween(at, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	providers, err := a.ProviderBreakdown(t.Context(), window)
	if err != nil || len(providers) != 1 || providers[0].FallbackRate != 50 {
		t.Fatalf("legacy fallback rate changed: %+v err=%v", providers, err)
	}
}

func TestFallbackIdentityIsIndependentOfExecutionCount(t *testing.T) {
	db := newCostTestDB(t)
	at := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	repo := NewRequests(db)
	cases := []struct {
		id                 string
		attempt, candidate int
		wantRate           float64
	}{
		{"explicit-primary", 4, 1, 0},
		{"explicit-fallback", 1, 3, 100},
		{"legacy-primary", 1, 0, 0},
		{"legacy-fallback", 3, 0, 100},
	}
	for _, tc := range cases {
		if err := repo.Insert(history.RequestRecord{
			ID: tc.id, Model: "model", Provider: tc.id, StartTime: at, Duration: time.Second,
			Attempt: tc.attempt, CandidatePosition: tc.candidate, UsageUnobserved: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	rows, total, err := repo.Query(t.Context(), RequestQuery{Page: 1, PageSize: 10})
	if err != nil || total != int64(len(cases)) {
		t.Fatalf("readback: total=%d err=%v", total, err)
	}
	for _, row := range rows {
		for _, tc := range cases {
			if row.ID == tc.id && (row.Attempt != tc.attempt || row.CandidatePosition != tc.candidate || !row.OutcomeKnown || row.CostKnown || row.DetailsKnown) {
				t.Fatalf("execution identity/unknown usage altered: %+v", row)
			}
		}
	}
	a := NewAnalytics(db)
	window, err := a.WindowBetween(at, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	providers, err := a.ProviderBreakdown(t.Context(), window)
	if err != nil || len(providers) != len(cases) {
		t.Fatalf("aggregate=%+v err=%v", providers, err)
	}
	for _, provider := range providers {
		for _, tc := range cases {
			if provider.Provider == tc.id && (provider.FallbackRate != tc.wantRate || provider.KnownRequests != 1) {
				t.Errorf("provider=%+v want fallback_rate=%v", provider, tc.wantRate)
			}
		}
	}
	if err := repo.Insert(history.RequestRecord{ID: "invalid", Model: "model", StartTime: at, CandidatePosition: -1}); err == nil {
		t.Fatal("negative candidate position accepted")
	}
}
