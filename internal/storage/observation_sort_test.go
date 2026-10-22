package storage

import (
	"github.com/routatic/proxy/internal/history"
	"testing"
	"time"
)

func TestMissingObservationsSortLast(t *testing.T) {
	db := newCostTestDB(t)
	repo := NewRequests(db)
	base := time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC)
	for _, rec := range []history.RequestRecord{
		{ID: "fast", Model: "m", Provider: "p", StartTime: base, FirstByte: 20 * time.Millisecond, ReasoningEffort: "low", Success: true},
		{ID: "missing", Model: "m", Provider: "p", StartTime: base, Success: true},
		{ID: "slow", Model: "m", Provider: "p", StartTime: base, FirstByte: 100 * time.Millisecond, ReasoningEffort: "xhigh", Success: true},
	} {
		insertCostRecord(t, db, rec)
	}
	insertCostRecord(t, db, history.RequestRecord{ID: "empty", Model: "m", Provider: "p", StartTime: base, Success: true})
	if _, err := db.DB().Exec("UPDATE requests SET reasoning_effort='' WHERE id='empty'"); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"first_byte_ms", "reasoning_effort"} {
		for _, order := range []string{"asc", "desc"} {
			t.Run(field+"/"+order, func(t *testing.T) {
				recs, _, err := repo.Query(RequestQuery{SortBy: field, SortOrder: order})
				if err != nil {
					t.Fatal(err)
				}
				ids := []string{recs[0].ID, recs[1].ID, recs[2].ID}
				wantFirst := "fast"
				if order == "desc" {
					wantFirst = "slow"
				}
				if ids[0] != wantFirst || (ids[1] != "fast" && ids[1] != "slow") {
					t.Errorf("order=%v, want known observations first with direction", ids)
				}
				if recs[2].ID != "missing" && recs[2].ID != "empty" {
					t.Fatal("missing observations not last")
				}
				if recs[3].ID != "missing" && recs[3].ID != "empty" {
					t.Fatal("missing observations not last")
				}
			})
		}
	}
}
