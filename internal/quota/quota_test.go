package quota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseNestedPercentOnlyDerivesDollars(t *testing.T) {
	rep, err := Parse([]byte(`{
		"plan": "go",
		"usage": {
			"rolling": {"status": "ok", "percent": 25, "resetsAt": "2026-09-04T12:00:00Z"},
			"weekly": {"percent": 50},
			"monthly": {"percent": 10}
		}
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rep.Plan != "go" {
		t.Errorf("plan = %q, want go", rep.Plan)
	}
	if rep.Rolling5h == nil || rep.Weekly == nil || rep.Monthly == nil {
		t.Fatal("all three windows must be present")
	}
	if !rep.Rolling5h.PercentDerived {
		t.Error("percent-only window must be flagged as derived")
	}
	if got := *rep.Rolling5h.UsedDollars; got != Rolling5hLimit*0.25 {
		t.Errorf("rolling used = %v, want %v", got, Rolling5hLimit*0.25)
	}
	if got := *rep.Weekly.LimitDollars; got != WeeklyLimit {
		t.Errorf("weekly limit = %v, want %v", got, WeeklyLimit)
	}
	if rep.Rolling5h.ResetsAt != "2026-09-04T12:00:00Z" {
		t.Errorf("resetsAt = %q", rep.Rolling5h.ResetsAt)
	}
}

func TestParseFlatDollarsAndResetKeyVariants(t *testing.T) {
	// resetInSec (one "s" short of resetsInSec) is the variant that exact-tag
	// matching drops; monthly uses the snake_case spelling.
	rep, err := Parse([]byte(`{
		"rolling5h": {"usageDollars": 3, "limitDollars": 12, "usagePercent": 25, "resetInSec": 3600},
		"week": {"usedDollars": 6, "limitDollars": 30, "resetsInSeconds": 86400},
		"month": {"usageDollars": 12, "limitDollars": 60, "reset_in_sec": 604800}
	}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rep.Rolling5h.ResetsInSec == nil || *rep.Rolling5h.ResetsInSec != 3600 {
		t.Errorf("rolling resetInSec not picked up: %+v", rep.Rolling5h.ResetsInSec)
	}
	if rep.Weekly.ResetsInSec == nil || *rep.Weekly.ResetsInSec != 86400 {
		t.Errorf("weekly resetsInSeconds not picked up")
	}
	if rep.Monthly.ResetsInSec == nil || *rep.Monthly.ResetsInSec != 604800 {
		t.Errorf("monthly reset_in_sec not picked up")
	}
	if rep.Rolling5h.PercentDerived {
		t.Error("a window with real dollars must not be flagged as derived")
	}
	// Weekly reports dollars without a percentage: the fill ratio is computed.
	if !rep.Weekly.HasPercent || rep.Weekly.UsedPercent != 20 {
		t.Errorf("weekly used percent = %v, want 20", rep.Weekly.UsedPercent)
	}
}

func TestParseRejectsResponseWithoutWindows(t *testing.T) {
	if _, err := Parse([]byte(`{"hello": "world"}`)); err == nil {
		t.Fatal("expected an error when no usage window is present")
	}
	if _, err := Parse([]byte(`not json`)); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

func TestUsageURL(t *testing.T) {
	cases := []struct {
		base    string
		want    string
		wantErr bool
	}{
		{"", DefaultUsageURL, false},
		{"https://opencode.ai/zen/go/v1/chat/completions", "https://opencode.ai/zen/go/v1/usage", false},
		{"https://mirror.example/zen/go/v1/messages", "https://mirror.example/zen/go/v1/usage", false},
		{"https://mirror.example/zen/go/v1/usage", "https://mirror.example/zen/go/v1/usage", false},
		{"https://mirror.example/custom", "", true},
	}
	for _, tc := range cases {
		got, err := UsageURL(tc.base)
		if tc.wantErr {
			if err == nil {
				t.Errorf("UsageURL(%q) = %q, want error", tc.base, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("UsageURL(%q): %v", tc.base, err)
			continue
		}
		if got != tc.want {
			t.Errorf("UsageURL(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
}

func TestFetchSendsBearerAndSurfacesStatus(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"rolling5h": {"usagePercent": 5}}`))
	}))
	defer srv.Close()

	rep, err := Fetch(context.Background(), srv.Client(), srv.URL, "sk-test-key")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotAuth != "Bearer sk-test-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if rep.Rolling5h == nil || rep.Rolling5h.UsedPercent != 5 {
		t.Errorf("unexpected report: %+v", rep.Rolling5h)
	}

	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer denied.Close()
	if _, err := Fetch(context.Background(), denied.Client(), denied.URL, "sk-bad"); err == nil {
		t.Fatal("expected an error for HTTP 401")
	}

	if _, err := Fetch(context.Background(), srv.Client(), srv.URL, "  "); err == nil {
		t.Fatal("expected an error when no key is provided")
	}
}
