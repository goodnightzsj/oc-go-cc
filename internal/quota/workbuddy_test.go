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
	"time"
)

const workBuddyOverviewFixture = `{"total":2,"healthy":1,"cooling":0,"disabled":0,"paused":1,"in_flight_full":0,"balance_refresh_sec":300,"balance_refreshed_at":"2026-10-24T12:00:00+08:00","accounts":[{"uid":"private-identity","nickname":"private-name","realm":"cn","credits":0,"credits_total":200,"paused":true,"reason":"private-error","access_token":"private-token"},{"realm":"global","credits":900000000,"credits_total":-1}]}`
const workBuddyCountsFixture = `"requests":3,"errors":1,"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"cache_hit_tokens":40,"cache_miss_tokens":60,"credits":0,"credit_samples":2,"avg_latency_ms":200,"avg_tokens_per_second":5`
const workBuddyUsageFixture = `{"totals":{` + workBuddyCountsFixture + `},"by_model":[{"key":"global:example","extra":"private-name",` + workBuddyCountsFixture + `}],"series":[{"t":"2026-10-24T12","scope":"hour",` + workBuddyCountsFixture + `}],"by_account":[{"key":"private-identity"}],"credit_by_account":[{"nickname":"private-name"}],"generated":"2026-10-24T12:30:00+08:00"}`

func TestWorkBuddyPanelURL(t *testing.T) {
	for _, path := range []string{"", "/", "/v1", "/v1/", "/v1/chat/completions/", "/mount/v1/chat/completions"} {
		want := "http://127.0.0.1:7863/panel/api"
		if strings.HasPrefix(path, "/mount/") {
			want = "http://127.0.0.1:7863/mount/panel/api"
		}
		got, err := WorkBuddyPanelURL(" http://127.0.0.1:7863" + path + " ")
		if err != nil || got != want {
			t.Fatalf("path %q: got %q, %v; want %q", path, got, err, want)
		}
	}
	for _, endpoint := range []string{"", "ftp://host/v1", "https://user:secret@host/v1", "https://host/v1?secret=1", "https://host/v1?", "https://host/v1#secret", "https://host/a%2fv1", "https://host/unrelated", "/v1"} {
		if got, err := WorkBuddyPanelURL(endpoint); err == nil || got != "" {
			t.Fatalf("unsafe endpoint accepted: %q", endpoint)
		}
	}
}

func TestWorkBuddyFetchAndPrivacy(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer synthetic-key" || r.Header.Get("Cookie") != "" {
			t.Error("not a key-only read")
		}
		switch r.URL.RequestURI() {
		case "/panel/api/overview":
			_, _ = fmt.Fprint(w, workBuddyOverviewFixture)
		case "/panel/api/usage?hours=24":
			_, _ = fmt.Fprint(w, workBuddyUsageFixture)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	report, err := FetchWorkBuddy(context.Background(), server.Client(), server.URL+"/panel/api", "synthetic-key")
	if err != nil || report == nil || calls.Load() != 2 {
		t.Fatalf("fetch: %v; calls=%d", err, calls.Load())
	}
	if *report.Overview.Accounts[0].Credits != 0 || *report.Overview.Accounts[1].CreditsTotal != -1 || *report.Usage.Totals.TotalTokens != 120 || *report.Usage.Totals.Credits != 0 || *report.Usage.Totals.CreditSamples != 2 || report.Usage.Hours != 24 || report.Usage.Series[0].Time != "2026-10-24T12" {
		t.Fatal("lost zero/unlimited/credit coverage or changed token/time semantics")
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-", "synthetic-key", "uid", "nickname", "access_token", "by_account", "extra"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("retained forbidden field: %s", forbidden)
		}
	}
	before := calls.Load()
	if _, err := FetchWorkBuddy(context.Background(), server.Client(), server.URL, " "); err == nil || calls.Load() != before {
		t.Fatal("empty key sent a request")
	}
}

func TestWorkBuddyFailures(t *testing.T) {
	for _, mode := range []string{"overview-denied", "usage-denied", "both-denied", "redirect", "oversize", "invalid-json", "missing-totals", "missing-count", "negative-count", "excess-errors", "invalid-time", "wrong-type", "missing-pool", "missing-accounts"} {
		t.Run(mode, func(t *testing.T) {
			var redirected atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
			defer target.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				overview := strings.HasSuffix(r.URL.Path, "/overview")
				if mode == "both-denied" || mode == "overview-denied" && overview || mode == "usage-denied" && !overview {
					w.WriteHeader(http.StatusForbidden)
					_, _ = fmt.Fprint(w, "private-echo synthetic-key")
					return
				}
				if mode == "redirect" {
					http.Redirect(w, r, target.URL, http.StatusFound)
					return
				}
				body := workBuddyUsageFixture
				if overview {
					body = workBuddyOverviewFixture
					if mode == "missing-pool" {
						body = `{}`
					}
					if mode == "missing-accounts" {
						body = `{"total":0,"healthy":0,"cooling":0,"disabled":0}`
					}
				} else {
					switch mode {
					case "missing-totals":
						body = `{}`
					case "missing-count":
						body = strings.ReplaceAll(body, `"requests":3`, `"requests":null`)
					case "negative-count":
						body = strings.ReplaceAll(body, `"requests":3`, `"requests":-1`)
					case "excess-errors":
						body = strings.ReplaceAll(body, `"errors":1`, `"errors":4`)
					case "invalid-time":
						body = strings.ReplaceAll(body, "2026-10-24T12", "private-invalid-time")
					case "wrong-type":
						body = strings.ReplaceAll(body, `"credits":0`, `"credits":"synthetic-key"`)
					}
				}
				if mode == "oversize" {
					body = strings.Repeat(" ", maxResponseBytes+1)
				}
				if mode == "invalid-json" {
					body = `{"private-echo":"synthetic-key"`
				}
				_, _ = fmt.Fprint(w, body)
			}))
			defer server.Close()
			report, err := FetchWorkBuddy(context.Background(), server.Client(), server.URL+"/panel/api", "synthetic-key")
			allFailed := mode == "both-denied" || mode == "redirect" || mode == "oversize" || mode == "invalid-json"
			if allFailed {
				if err == nil || report != nil {
					t.Fatal("complete failure looked successful")
				}
			} else if err != nil || report == nil {
				t.Fatalf("partial failure discarded good data: %v", err)
			} else if mode == "overview-denied" || mode == "missing-pool" || mode == "missing-accounts" {
				if report.Overview != nil || report.OverviewError == "" || report.Usage == nil {
					t.Fatal("overview failure not preserved independently")
				}
			} else if report.Usage != nil || report.UsageError == "" || report.Overview == nil {
				t.Fatal("usage failure not preserved independently")
			}
			encoded, _ := json.Marshal(report)
			message := string(encoded) + fmt.Sprint(err)
			if redirected.Load() != 0 || strings.Contains(message, "private-") || strings.Contains(message, "synthetic-key") {
				t.Fatal("redirect followed or error content leaked")
			}
		})
	}
}

func TestWorkBuddyCancellation(t *testing.T) {
	started := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := FetchWorkBuddy(ctx, server.Client(), server.URL+"/panel/api", "synthetic-key")
		done <- err
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("account request did not start")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || strings.Contains(err.Error(), "synthetic-key") {
			t.Fatal("cancellation hidden or credential leaked")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled requests did not finish")
	}
}
