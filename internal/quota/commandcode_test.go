package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const commandCodeCreditsFixture = `{"credits":{"freeCredits":0,"monthlyCredits":70,"purchasedCredits":0},"windowLimits":{"limited":true,"exceeded":null,"fiveHour":{"used":0,"cap":14,"exceeded":false,"resetAt":0},"weekly":{"used":10,"cap":35,"exceeded":false,"resetAt":1910000000000}}}`
const commandCodeSubscriptionFixture = `{"success":true,"data":{"planId":"individual-goat","status":"active","currentPeriodStart":"2026-09-10T07:35:57.000Z","currentPeriodEnd":"2026-10-10T07:35:57.000Z","cancelAtPeriodEnd":false,"userId":"private-account-id","customerId":"private-payment-id"}}`
const commandCodeUsageFixture = `{"totalCount":0,"completedCount":0,"failedCount":0,"totalTokensIn":0,"totalTokensOut":0,"totalTokens":0,"totalCredits":0,"periodBasis":"billing-period"}`

func TestCommandCodeURLAndAccountContract(t *testing.T) {
	for _, endpoint := range []string{"https://api.commandcode.ai/provider/v1/chat/completions", "https://mirror.invalid/mount/provider/v1/messages/", "http://127.0.0.1:3456/provider/v1"} {
		got, err := CommandCodeBaseURL(endpoint)
		want := strings.Split(endpoint, "/provider/v1")[0] + "/alpha"
		if err != nil || got != want {
			t.Fatalf("base %q = %q, %v; want %q", endpoint, got, err, want)
		}
	}
	for _, endpoint := range []string{"", "ftp://mirror.invalid/provider/v1", "https://user:pass@mirror.invalid/provider/v1", "https://mirror.invalid/provider/v1?key=secret", "https://mirror.invalid/provider/v1?", "https://mirror.invalid/provider/v1#fragment", "https://mirror.invalid/other/v1", "https://mirror.invalid/provider%2fv1"} {
		if got, err := CommandCodeBaseURL(endpoint); err == nil || got != "" {
			t.Fatalf("unsafe or unrelated base derived an account endpoint: %q", endpoint)
		}
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer synthetic-key" || r.Header.Get("Cookie") != "" {
			t.Error("account query did not use the explicit key-only GET contract")
		}
		switch r.URL.Path {
		case "/alpha/billing/credits":
			_, _ = fmt.Fprint(w, commandCodeCreditsFixture)
		case "/alpha/billing/subscriptions":
			_, _ = fmt.Fprint(w, commandCodeSubscriptionFixture)
		case "/alpha/usage/summary":
			_, _ = fmt.Fprint(w, commandCodeUsageFixture)
		default:
			t.Errorf("unexpected account path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	report, err := FetchCommandCode(context.Background(), server.Client(), server.URL+"/alpha", "synthetic-key")
	if err != nil || report == nil || calls.Load() != 3 {
		t.Fatalf("account report: %v, calls=%d", err, calls.Load())
	}
	if *report.Credits.Credits.MonthlyCredits != 70 || *report.Credits.WindowLimits.FiveHour.Used != 0 || *report.Credits.WindowLimits.FiveHour.ResetAt != 0 || *report.Credits.WindowLimits.Weekly.ResetAt != 1910000000000 || report.Subscription.PlanID != "individual-goat" || *report.Usage.TotalCount != 0 {
		t.Fatal("official credits, plan, known zero or millisecond reset was changed")
	}
	encoded, _ := json.Marshal(report)
	for _, forbidden := range []string{"private-", "monthlyCreditsGranted", "synthetic-key", "customerId", "userId"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("account response retained forbidden data: %s", forbidden)
		}
	}
}

func TestCommandCodePartialFailuresAndSafety(t *testing.T) {
	for _, mode := range []string{"subscription-denied", "no-subscription", "missing-credit", "null-credit", "wrong-type", "invalid-window", "invalid-usage", "invalid-period", "all-denied", "redirect", "oversize", "invalid-json"} {
		t.Run(mode, func(t *testing.T) {
			var redirected atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
			defer target.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "all-denied" || mode == "subscription-denied" && strings.HasSuffix(r.URL.Path, "/subscriptions") {
					w.WriteHeader(http.StatusForbidden)
					_, _ = fmt.Fprint(w, r.Header.Get("Authorization"))
					return
				}
				if mode == "redirect" {
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
					return
				}
				body := commandCodeCreditsFixture
				switch {
				case strings.HasSuffix(r.URL.Path, "/subscriptions"):
					body = commandCodeSubscriptionFixture
					switch mode {
					case "no-subscription":
						body = `{"success":true,"data":null}`
					case "invalid-period":
						body = strings.ReplaceAll(body, "2026-09-10T07:35:57.000Z", "not-a-date")
					}
				case strings.HasSuffix(r.URL.Path, "/summary"):
					body = commandCodeUsageFixture
					if mode == "invalid-usage" {
						body = `{"periodBasis":"billing-period"}`
					}
				default:
					switch mode {
					case "missing-credit":
						body = `{"credits":{"monthlyCredits":70}}`
					case "null-credit":
						body = strings.ReplaceAll(body, `"monthlyCredits":70`, `"monthlyCredits":null`)
					case "wrong-type":
						body = strings.ReplaceAll(body, `"monthlyCredits":70`, `"monthlyCredits":"synthetic-key"`)
					case "invalid-window":
						body = strings.ReplaceAll(body, `"cap":14`, `"cap":-1`)
					case "oversize":
						body = strings.Repeat(" ", maxResponseBytes+1)
					case "invalid-json":
						body = `{"credits":`
					}
				}
				_, _ = fmt.Fprint(w, body)
			}))
			defer server.Close()
			report, err := FetchCommandCode(context.Background(), server.Client(), server.URL+"/alpha", "synthetic-key")
			if mode == "all-denied" || mode == "redirect" {
				if report != nil || err == nil || strings.Contains(err.Error(), "synthetic-key") || redirected.Load() != 0 {
					t.Fatalf("unsafe failure handling: report=%v err=%v", report, err)
				}
				return
			}
			if err != nil || report == nil {
				t.Fatalf("one failed block hid the successful blocks: %v", err)
			}
			switch mode {
			case "no-subscription":
				if report.Subscription != nil || report.SubscriptionError != "" || report.Credits == nil || report.Usage == nil {
					t.Fatal("a successful null subscription was misrepresented")
				}
			case "subscription-denied", "invalid-period":
				if report.Subscription != nil || report.SubscriptionError == "" || report.Credits == nil || report.Usage == nil {
					t.Fatal("subscription failure lost its scope")
				}
			case "invalid-usage":
				if report.Usage != nil || report.UsageError == "" || report.Credits == nil || report.Subscription == nil {
					t.Fatal("missing usage fields became successful zeroes")
				}
			default:
				if report.Credits != nil || report.CreditsError == "" || report.Usage == nil || report.Subscription == nil {
					t.Fatal("invalid credits hid other blocks or became a valid zero balance")
				}
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "synthetic-key") {
				t.Fatal("an echoed secret escaped through an error response")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if report, err := FetchCommandCode(ctx, nil, "http://127.0.0.1:1/alpha", "synthetic-key"); err == nil || report != nil {
		t.Fatal("cancelled account query succeeded")
	}
}
