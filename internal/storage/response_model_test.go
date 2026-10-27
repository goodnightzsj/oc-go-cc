package storage

import (
	"reflect"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestResponseModelAttributionRoundTripSearchAndSort(t *testing.T) {
	repo := NewRequests(newCostTestDB(t))
	fixtures := []history.RequestRecord{
		{ID: "response", Model: "a-route", ResponseModel: "vmc/served", UpstreamModel: "deepseek/canonical", UpstreamChannel: "deepseek", Provider: "cline-pass"},
		{ID: "canonical", Model: "z-route", UpstreamModel: "b-canonical", Provider: "cline-pass"},
		{ID: "legacy", Model: "c-route", Provider: "workbuddy"},
	}
	for i := range fixtures {
		fixtures[i].StartTime = time.Date(2026, 10, 27, 0, i, 0, 0, time.UTC)
		fixtures[i].Success = true
		if err := repo.Insert(fixtures[i]); err != nil {
			t.Fatal(err)
		}
	}
	for _, term := range []string{"VMC/served", "deepseek/canonical", "deepseek"} {
		rows, total, err := repo.Query(t.Context(), RequestQuery{Search: term})
		if err != nil || total != 1 {
			t.Fatalf("search %q: total=%d err=%v", term, total, err)
		}
		row := rows[0]
		if row.Model != fixtures[0].Model || row.Provider != "cline-pass" || row.ResponseModel != "vmc/served" || row.UpstreamModel != "deepseek/canonical" || row.UpstreamChannel != "deepseek" {
			t.Fatalf("source axes merged: %+v", row)
		}
	}
	for _, order := range []string{"asc", "desc"} {
		rows, _, err := repo.Query(t.Context(), RequestQuery{SortBy: "model", SortOrder: order})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, row := range rows {
			ids = append(ids, row.ID)
			if row.ID != "response" && row.ResponseModel != "" {
				t.Fatal("unobserved response model was fabricated")
			}
		}
		want := []string{"canonical", "legacy", "response"}
		if order == "desc" {
			want = []string{"response", "legacy", "canonical"}
		}
		if !reflect.DeepEqual(ids, want) {
			t.Fatalf("%s sort=%v want=%v", order, ids, want)
		}
	}
	// Exact model filters and summaries retain the route/billing identity.
	_, total, err := repo.Query(t.Context(), RequestQuery{Model: "a-route"})
	if err != nil || total != 1 {
		t.Fatalf("billing model filter: total=%d err=%v", total, err)
	}
}
