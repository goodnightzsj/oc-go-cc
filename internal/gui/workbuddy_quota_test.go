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

func TestWorkBuddyQuotaScopeAndCache(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.Header.Get("Cookie") != "" || !strings.HasPrefix(r.URL.Path, "/panel/api/") {
			t.Error("not a read-only WorkBuddy request")
		}
		switch r.Header.Get("Authorization") {
		case "Bearer synthetic-good":
		case "Bearer synthetic-bad":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, "synthetic-bad")
			return
		default:
			t.Error("unrelated credential sent to WorkBuddy")
		}
		if r.URL.Path == "/panel/api/overview" {
			_, _ = fmt.Fprint(w, `{"total":1,"healthy":1,"cooling":0,"disabled":0,"accounts":[{"uid":"private-id","nickname":"private-name","realm":"cn","credits":100}]}`)
		} else if r.URL.RequestURI() == "/panel/api/usage?hours=24" {
			w.WriteHeader(http.StatusNotImplemented)
		} else {
			t.Errorf("unexpected endpoint: %s", r.URL)
		}
	}))
	defer upstream.Close()
	srv, _ := configTestServer(t, fmt.Sprintf(`{"api_key":"synthetic-global","workbuddy":{"base_url":%q,"api_keys":["synthetic-good","synthetic-bad","synthetic-good"]}}`, upstream.URL+"/v1/chat/completions"))
	call := func(query string) quotaResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.handleQuota(rec, httptest.NewRequest(http.MethodGet, "/api/quota?provider=workbuddy"+query, nil))
		var result quotaResponse
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil {
			t.Fatalf("quota response: HTTP %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "synthetic-") || strings.Contains(rec.Body.String(), "private-") || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private data leaked or browser caching allowed")
		}
		return result
	}
	result := call("")
	if result.Provider != "workbuddy" || result.Status != "partial" || result.Source != "gateway_api" || result.Currency != "" || len(result.Accounts) != 2 || calls.Load() != 4 {
		t.Fatalf("wrong scope/status/count: %+v calls=%d", result, calls.Load())
	}
	if result.Accounts[0].WorkBuddy == nil || result.Accounts[0].WorkBuddy.UsageError == "" || result.Accounts[1].Error == "" || result.ModelLimits != nil || len(result.ModelUsage) != 0 || result.Accounts[0].Ledger != nil {
		t.Fatal("lost partial data or mixed local ledger/Go allowance")
	}
	if len(result.Links) != 1 || result.Links[0].URL != upstream.URL+"/panel/" {
		t.Fatal("missing same-instance panel link")
	}
	if !call("").Cached || calls.Load() != 4 {
		t.Fatal("poll bypassed cache")
	}
	call("&refresh=1")
	if calls.Load() != 8 {
		t.Fatal("manual refresh failed")
	}
	cfg := *srv.atomicCfg.Get()
	cfg.WorkBuddy.APIKeys = []string{"synthetic-bad"}
	srv.atomicCfg.ApplyLoaded(&cfg)
	if got := call(""); got.Cached || got.Status != "error" || calls.Load() != 10 {
		t.Fatal("changed key reused previous cache")
	}
	var otherCalls atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherCalls.Add(1)
		w.WriteHeader(http.StatusNotImplemented)
	}))
	defer other.Close()
	cfg.WorkBuddy.BaseURL = other.URL + "/v1"
	srv.atomicCfg.ApplyLoaded(&cfg)
	if got := call(""); got.Cached || got.Endpoint != other.URL+"/panel/api" || otherCalls.Load() != 2 || calls.Load() != 10 {
		t.Fatal("changed instance reused previous cache")
	}
	cfg.WorkBuddy.APIKeys = nil
	srv.atomicCfg.ApplyLoaded(&cfg)
	if got := call(""); got.Cached || got.Status != "not_configured" || len(got.Accounts) != 0 || calls.Load() != 10 || otherCalls.Load() != 2 {
		t.Fatal("key removal reused data or queried with global key")
	}
}
