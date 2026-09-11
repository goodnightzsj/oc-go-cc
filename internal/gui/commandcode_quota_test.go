package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCommandCodeQuotaScopeAndCache(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/gateway/alpha/") || r.Header.Get("Cookie") != "" {
			t.Error("quota used a wrong method, origin prefix or browser credential")
		}
		if r.Header.Get("Authorization") == "Bearer synthetic-command-bad" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, r.Header.Get("Authorization"))
			return
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-command-good" {
			t.Error("an unrelated key was sent to CommandCode")
		}
		switch r.URL.Path {
		case "/gateway/alpha/billing/credits":
			fmt.Fprint(w, `{"credits":{"freeCredits":0,"monthlyCredits":60,"purchasedCredits":2},"windowLimits":{"limited":true,"exceeded":null,"fiveHour":{"used":3,"cap":14,"resetAt":0,"exceeded":false},"weekly":{"used":10,"cap":35,"resetAt":0,"exceeded":false}}}`)
		case "/gateway/alpha/billing/subscriptions":
			fmt.Fprint(w, `{"success":true,"data":{"planId":"individual-goat","status":"active","currentPeriodStart":"2026-09-10T07:35:57.000Z","currentPeriodEnd":"2026-10-10T07:35:57.000Z","cancelAtPeriodEnd":false,"userId":"private-identity"}}`)
		case "/gateway/alpha/usage/summary":
			fmt.Fprint(w, `{"totalCount":2,"totalCost":5,"averageCost":2.5,"successRate":0.5,"completedCount":1,"failedCount":1,"totalTokensIn":100,"totalTokensOut":10,"totalTokens":110,"totalCredits":10,"totalFreeCredits":0,"totalMonthlyCredits":10,"totalPurchasedCredits":0,"periodBasis":"billing-period"}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	srv, _ := configTestServer(t, fmt.Sprintf(`{"api_key":"synthetic-global","commandcode":{"base_url":%q,"api_keys":["synthetic-command-good","synthetic-command-bad","synthetic-command-good"]}}`, upstream.URL+"/gateway/provider/v1/chat/completions"))
	call := func(query string) map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.handleQuota(rec, httptest.NewRequest(http.MethodGet, "/api/quota?provider=commandcode"+query, nil))
		var result map[string]any
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil {
			t.Fatalf("quota response: HTTP %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "synthetic-") || strings.Contains(rec.Body.String(), "private-identity") || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("quota exposed a credential/account identity or allowed browser caching")
		}
		return result
	}
	result := call("")
	if result["provider"] != "commandcode" || result["status"] != "partial" || result["source"] != "official_alpha_api" || result["currency"] != "USD" {
		t.Fatalf("wrong provenance/status: %v", result)
	}
	accounts := result["accounts"].([]any)
	if len(accounts) != 2 || calls.Load() != 6 {
		t.Fatalf("expected two distinct keys and three reads per key; accounts=%d calls=%d", len(accounts), calls.Load())
	}
	first, second := accounts[0].(map[string]any), accounts[1].(map[string]any)
	if first["commandcode"] == nil || second["error"] == nil || result["model_limits"] != nil || len(result["model_usage"].([]any)) != 0 {
		t.Fatal("successful account, key failure or Go-model separation was lost")
	}
	if !call("")["cached"].(bool) || calls.Load() != 6 {
		t.Fatal("same-scope polling bypassed the cache")
	}
	call("&refresh=1")
	if calls.Load() != 12 {
		t.Fatal("manual refresh did not query exactly this platform")
	}
	cfg := *srv.atomicCfg.Get()
	cfg.CommandCode.APIKeys = nil
	cfg.CommandCode.APIKey = ""
	srv.atomicCfg.ApplyLoaded(&cfg)
	result = call("")
	if result["status"] != "not_configured" || len(result["accounts"].([]any)) != 0 || calls.Load() != 12 || result["cached"] != false {
		t.Fatal("key removal reused account data or sent a global key")
	}
}
