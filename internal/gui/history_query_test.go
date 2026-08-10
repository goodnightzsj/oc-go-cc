package gui

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestHistoryRequestQueryParsesServerSideFilters(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/history?search=quota&model=m&provider=p&scenario=s&cost_source=provider&start=2026-08-01&end=2026-08-08&success=false&streaming=true&sort=duration_ms&order=asc", nil)
	q, err := historyRequestQuery(req, 2, 25)
	if err != nil {
		t.Fatalf("historyRequestQuery: %v", err)
	}
	if q.Page != 2 || q.PageSize != 25 || q.Search != "quota" || q.Model != "m" || q.Provider != "p" || q.Scenario != "s" || q.CostSource != "provider" {
		t.Fatalf("query fields = %+v", q)
	}
	if q.Success == nil || *q.Success || q.Streaming == nil || !*q.Streaming {
		t.Fatalf("boolean filters = success %v, streaming %v", q.Success, q.Streaming)
	}
	if q.Start == nil || q.End == nil || q.End.Sub(*q.Start) != 8*24*time.Hour {
		t.Fatalf("date range = %v .. %v, want inclusive Aug 1-8", q.Start, q.End)
	}
	if q.SortBy != "duration_ms" || q.SortOrder != "asc" {
		t.Fatalf("sort = %q %q", q.SortBy, q.SortOrder)
	}
}

func TestHistoryRequestQueryRejectsInvalidBoolean(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/history?success=maybe", nil)
	if _, err := historyRequestQuery(req, 1, 50); err == nil {
		t.Fatal("historyRequestQuery accepted invalid success filter")
	}
}

func TestHistoryRequestQueryRejectsInvalidCostSource(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/history?cost_source=guessed", nil)
	if _, err := historyRequestQuery(req, 1, 50); err == nil {
		t.Fatal("historyRequestQuery accepted invalid cost source")
	}
}
