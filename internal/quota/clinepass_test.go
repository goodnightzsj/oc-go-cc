package quota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClinePassUsageURLKeepsOriginAndRejectsForeignPath(t *testing.T) {
	got, err := ClinePassUsageURL("https://api.cline.bot/api/v1/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://api.cline.bot/api/v1/users/me/plan/usage-limits" {
		t.Errorf("derived %q", got)
	}
	// A mirror URL must not have its key sent to the public host.
	for _, bad := range []string{
		"https://mirror.internal/v1/chat/completions",
		"https://api.cline.bot/other",
		"http://user:pass@api.cline.bot/api/v1/chat/completions",
		"https://api.cline.bot/api/v1/chat/completions?x=1",
	} {
		if _, err := ClinePassUsageURL(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// clinePassEndpoint derives the endpoint exactly as the quota handler does, so
// these tests exercise the real handoff: derive once, then fetch the derived
// value. Passing a base URL straight in would let FetchClinePass and
// ClinePassUsageURL disagree about what they accept without any test noticing,
// which is precisely how the panel ended up reporting a credential error
// against a correctly-derived endpoint.
func clinePassEndpoint(t *testing.T, base string) string {
	t.Helper()
	endpoint, err := ClinePassUsageURL(base + "/api/v1/chat/completions")
	if err != nil {
		t.Fatalf("derive endpoint: %v", err)
	}
	return endpoint
}

func clinePassServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer cline-key" {
			t.Errorf("credential not sent: %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

// The endpoint returns nanosecond-precision timestamps, which RFC3339 alone
// rejects - parsing with it would report every window as broken.
func TestClinePassParsesNanosecondReset(t *testing.T) {
	page := clinePassServer(t, http.StatusOK, `{"success":true,"data":{"limits":[
		{"type":"five_hour","percentUsed":42.5,"resetsAt":"2026-09-08T17:00:44.598174595Z"},
		{"type":"weekly","percentUsed":10,"resetsAt":null},
		{"type":"monthly","percentUsed":3.25}
	]}}`)
	report, err := FetchClinePass(context.Background(), page.Client(), clinePassEndpoint(t, page.URL), "cline-key")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Windows) != 3 {
		t.Fatalf("got %d windows, want 3", len(report.Windows))
	}
	if w := report.Windows[0]; w.Type != "five_hour" || w.PercentUsed != 42.5 || w.ResetsAt == nil {
		t.Errorf("five_hour = %+v", w)
	}
	// A null reset time is "unknown", not an error.
	if report.Windows[1].ResetsAt != nil {
		t.Errorf("weekly reset = %v, want nil for a null timestamp", report.Windows[1].ResetsAt)
	}
}

// An unknown window type is carried through: the platform may add one, and
// dropping it would hide a limit that is actually being enforced.
func TestClinePassKeepsUnknownWindowTypes(t *testing.T) {
	page := clinePassServer(t, http.StatusOK, `{"success":true,"data":{"limits":[
		{"type":"daily","percentUsed":7,"resetsAt":"2026-09-08T17:00:44Z"}
	]}}`)
	report, err := FetchClinePass(context.Background(), page.Client(), clinePassEndpoint(t, page.URL), "cline-key")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Windows) != 1 || report.Windows[0].Type != "daily" {
		t.Errorf("unknown window was dropped: %+v", report.Windows)
	}
}

// A response that reports failure, or that omits the numbers, must not be read
// as "no usage". Zero would show an idle account that is actually capped.
func TestClinePassRejectsUnusableResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantErr    bool
	}{
		{"explicit failure", `{"success":false,"error":{"message":"nope"}}`, true},
		{"no success flag", `{"data":{"limits":[]}}`, true},
		{"no data", `{"success":true}`, true},
		{"limit without percent", `{"success":true,"data":{"limits":[{"type":"weekly"}]}}`, true},
		{"limit without type", `{"success":true,"data":{"limits":[{"percentUsed":5}]}}`, true},
		{"bad timestamp", `{"success":true,"data":{"limits":[{"type":"weekly","percentUsed":5,"resetsAt":"soon"}]}}`, true},
		{"not json", `<html>gateway</html>`, true},
		{"empty limits", `{"success":true,"data":{"limits":[]}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := clinePassServer(t, http.StatusOK, tc.body)
			_, err := FetchClinePass(context.Background(), page.Client(), clinePassEndpoint(t, page.URL), "cline-key")
			if tc.wantErr && err == nil {
				t.Fatal("unusable response was accepted")
			}
		})
	}
}

func TestClinePassReportsHTTPFailure(t *testing.T) {
	page := clinePassServer(t, http.StatusTooManyRequests, "Try again in 2h")
	_, err := FetchClinePass(context.Background(), page.Client(), clinePassEndpoint(t, page.URL), "cline-key")
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("error = %v, want an HTTP 429 report", err)
	}
}
