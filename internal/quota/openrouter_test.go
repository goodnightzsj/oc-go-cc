package quota

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOpenRouterURLs(t *testing.T) {
	for _, base := range []string{"https://openrouter.ai/api/v1", "https://openrouter.ai/api/v1/chat/completions/", "http://localhost:3456/proxy/v1"} {
		key, credits, err := OpenRouterURLs(base)
		prefix := strings.TrimSuffix(strings.TrimRight(base, "/"), "/chat/completions")
		if err != nil || key != prefix+"/key" || credits != prefix+"/credits" {
			t.Errorf("%q = %q, %q, %v", base, key, credits, err)
		}
	}
	for _, base := range []string{"", "ftp://mirror.test/v1", "https://user:pass@mirror.test/v1", "https://mirror.test/", "https://mirror.test/v1?key=secret", "https://mirror.test/v1?", "https://mirror.test/v1#fragment"} {
		if key, credits, err := OpenRouterURLs(base); err == nil || key != "" || credits != "" {
			t.Errorf("invalid base %q derived an endpoint", base)
		}
	}
}

func TestOpenRouterKeyQuota(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
		limit      *float64
	}{
		{"finite", `{"data":{"limit":10,"limit_remaining":8,"limit_reset":"monthly","usage":2,"usage_daily":0}}`, true, floatPtr(10)},
		{"uncapped", `{"data":{"limit":null,"limit_remaining":null,"limit_reset":null,"usage":0}}`, true, nil},
		{"missing data", `{}`, false, nil},
		{"null data", `{"data":null}`, false, nil},
		{"missing cap", `{"data":{"usage":0}}`, false, nil},
		{"null usage", `{"data":{"limit":null,"limit_remaining":null,"limit_reset":null,"usage":null}}`, false, nil},
		{"wrong type", `{"data":{"limit":"unlimited","limit_remaining":null,"limit_reset":null,"usage":0}}`, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer synthetic-key" {
					t.Error("missing provider credential")
				}
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer ts.Close()
			got, err := FetchOpenRouterKey(context.Background(), ts.Client(), ts.URL, "synthetic-key")
			if (err == nil) != tc.valid {
				t.Fatalf("result = %+v, %v", got, err)
			}
			if !tc.valid {
				return
			}
			if (got.Limit == nil) != (tc.limit == nil) || tc.limit != nil && *got.Limit != *tc.limit {
				t.Errorf("limit = %v, want %v", got.Limit, tc.limit)
			}
			if got.UsageMonthly != nil || got.BYOKUsage != nil {
				t.Error("missing usage became a known zero")
			}
			if tc.name == "finite" && (got.UsageDaily == nil || *got.UsageDaily != 0) {
				t.Error("known zero daily usage was lost")
			}
		})
	}
}

func TestOpenRouterCreditsAndFailureSafety(t *testing.T) {
	var redirected atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/credits":
			_, _ = fmt.Fprint(w, `{"data":{"total_credits":1,"total_usage":2}}`)
		case "/missing":
			_, _ = fmt.Fprint(w, `{"data":{"total_credits":1}}`)
		case "/redirect":
			http.Redirect(w, r, target.URL, http.StatusFound)
		case "/oversize":
			_, _ = fmt.Fprint(w, strings.Repeat(" ", maxResponseBytes+1))
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, r.Header.Get("Authorization"))
		}
	}))
	defer ts.Close()
	credits, err := FetchOpenRouterCredits(context.Background(), ts.Client(), ts.URL+"/credits", "synthetic-management")
	if err != nil || credits.TotalCredits-credits.TotalUsage != -1 {
		t.Fatalf("negative balance must be retained: %+v, %v", credits, err)
	}
	for _, path := range []string{"/missing", "/forbidden", "/redirect", "/oversize"} {
		if _, err := FetchOpenRouterCredits(context.Background(), ts.Client(), ts.URL+path, "synthetic-management"); err == nil || strings.Contains(err.Error(), "synthetic-management") {
			t.Errorf("%s: expected safe explicit error, got %v", path, err)
		}
	}
	if redirected.Load() != 0 {
		t.Fatal("quota request followed a redirect")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := FetchOpenRouterCredits(ctx, ts.Client(), ts.URL+"/credits", "synthetic-management"); err == nil {
		t.Error("cancelled quota request succeeded")
	}
}

func floatPtr(value float64) *float64 { return &value }
