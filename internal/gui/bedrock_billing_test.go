package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/quota"
)

func TestBedrockBillingConfigPatch(t *testing.T) {
	initial := `{"api_key":"synthetic-global","aws_bedrock":{"api_key":"synthetic-bedrock","timeout_ms":5000,"billing":{"enabled":false,"profile":"billing-test","linked_account_id":"123456789012"}},"commandcode":{"api_key":"synthetic-command"}}`
	srv, path := configTestServer(t, initial)
	patch := `{"aws_bedrock":{"billing":{"enabled":true}}}`
	rec := httptest.NewRecorder()
	srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("billing patch failed: %d %s", rec.Code, rec.Body)
	}
	cfg := srv.atomicCfg.Get()
	if !cfg.AWSBedrock.Billing.Enabled || cfg.AWSBedrock.Billing.Profile != "billing-test" || cfg.AWSBedrock.Billing.LinkedAccountID != "123456789012" || cfg.AWSBedrock.APIKey != "synthetic-bedrock" || cfg.AWSBedrock.TimeoutMs != 5000 || cfg.CommandCode.APIKey != "synthetic-command" {
		t.Fatal("billing patch lost sibling configuration or modified inference")
	}
	rec = httptest.NewRecorder()
	srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodGet, "/api/proxy/config", nil))
	if strings.Contains(rec.Body.String(), "synthetic-") || !strings.Contains(rec.Body.String(), `"profile":"billing-test"`) {
		t.Fatal("billing settings GET leaked inference credentials or lost the profile")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(`{"aws_bedrock":{"billing":{"linked_account_id":""}}}`)))
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest || string(before) != string(after) || srv.atomicCfg.Get() != cfg {
		t.Fatal("unscoped billing patch changed disk or runtime")
	}
}

func TestBedrockBillingRequiresSeparateManualAction(t *testing.T) {
	srv, _ := configTestServer(t, `{"api_key":"synthetic-global","aws_bedrock":{"api_key":"synthetic-inference","billing":{"profile":"billing-readonly","linked_account_id":"123456789012"}}}`)
	calls := 0
	srv.fetchBedrockBilling = func(_ context.Context, cfg config.AWSBillingConfig) (*quota.BedrockBilling, error) {
		calls++
		if cfg.Profile != "billing-readonly" || cfg.LinkedAccountID != "123456789012" {
			t.Error("incorrect billing identity or account")
		}
		total := 1.25
		return &quota.BedrockBilling{LinkedAccountID: cfg.LinkedAccountID, TotalCost: &total, Currency: "EUR", Services: []string{"Amazon Bedrock"}, Daily: []quota.BedrockDailyCost{}}, nil
	}
	read := func(query string) quotaResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		method := http.MethodGet
		if strings.Contains(query, "billing_refresh=1") {
			method = http.MethodPost
		}
		srv.handleQuota(rec, httptest.NewRequest(method, "/api/quota?provider=aws-bedrock"+query, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected quota status: %d", rec.Code)
		}
		var result quotaResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if got := read("&billing_refresh=1"); calls != 0 || got.Reason != "aws_billing_disabled" || got.Status != "not_configured" {
		t.Fatal("disabled billing was queried")
	}
	cfg := *srv.atomicCfg.Get()
	cfg.AWSBedrock.Billing.Enabled = true
	srv.atomicCfg.ApplyLoaded(&cfg)
	for _, query := range []string{"", "&refresh=1"} {
		if got := read(query); calls != 0 || got.Reason != "aws_billing_refresh_required" {
			t.Fatal("platform navigation or legacy refresh initiated a paid query")
		}
	}
	got := read("&billing_refresh=1")
	if calls != 1 || got.Status != "available" || got.Currency != "EUR" || got.BedrockBilling == nil || got.Source != "official_api" || len(got.Accounts) != 0 || got.TTLSeconds != 86400 {
		t.Fatalf("unexpected billing response: %+v", got)
	}
	// Billing data is slow-moving: the ordinary 30-second quota TTL must not
	// turn polling into extra paid queries or discard this snapshot.
	entry := srv.quotaCache["aws-bedrock"]
	entry.Response.FetchedAt = time.Now().Add(-time.Hour)
	srv.quotaCache["aws-bedrock"] = entry
	if got := read("&refresh=1"); calls != 1 || !got.Cached || got.BedrockBilling == nil {
		t.Fatal("ordinary refresh did not use the daily billing snapshot")
	}
	cfg2 := cfg
	cfg2.AWSBedrock.Billing.LinkedAccountID = "234567890123"
	srv.atomicCfg.ApplyLoaded(&cfg2)
	if got := read(""); calls != 1 || got.BedrockBilling != nil || got.Reason != "aws_billing_refresh_required" {
		t.Fatal("account change reused another account's bill")
	}
	srv.atomicCfg.ApplyLoaded(&cfg)
	srv.fetchBedrockBilling = func(context.Context, config.AWSBillingConfig) (*quota.BedrockBilling, error) {
		return nil, errors.New("synthetic access denied")
	}
	if got := read("&billing_refresh=1"); got.Status != "error" || got.BedrockBilling != nil || got.Error != "synthetic access denied" {
		t.Fatal("failed refresh kept a stale or successful bill")
	}
}

func TestBedrockBillingPaidQueryUsesPost(t *testing.T) {
	srv, _ := configTestServer(t, `{"api_key":"synthetic-global","aws_bedrock":{"billing":{"enabled":true,"linked_account_id":"123456789012"}}}`)
	calls := 0
	srv.fetchBedrockBilling = func(context.Context, config.AWSBillingConfig) (*quota.BedrockBilling, error) {
		calls++
		total := 1.0
		return &quota.BedrockBilling{TotalCost: &total, Currency: "USD"}, nil
	}
	for _, tc := range []struct {
		name, method, query, origin, site string
		want                              int
	}{
		{"GET cannot spend", http.MethodGet, "provider=aws-bedrock&billing_refresh=1", "", "", http.StatusMethodNotAllowed},
		{"POST needs explicit action", http.MethodPost, "provider=aws-bedrock", "", "", http.StatusMethodNotAllowed},
		{"other providers remain read only", http.MethodPost, "provider=opencode-go&billing_refresh=1", "", "", http.StatusMethodNotAllowed},
		{"cross origin", http.MethodPost, "provider=aws-bedrock&billing_refresh=1", "https://other.invalid", "", http.StatusForbidden},
		{"cross site", http.MethodPost, "provider=aws-bedrock&billing_refresh=1", "", "cross-site", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://dashboard.invalid/api/quota?"+tc.query, nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			rec := httptest.NewRecorder()
			srv.handleQuota(rec, r)
			if rec.Code != tc.want || calls != 0 {
				t.Fatalf("request boundary = %d, upstream calls = %d; want %d and no paid request", rec.Code, calls, tc.want)
			}
		})
	}
	r := httptest.NewRequest(http.MethodPost, "http://dashboard.invalid/api/quota?provider=aws-bedrock&billing_refresh=1", nil)
	r.Header.Set("Origin", "http://dashboard.invalid")
	rec := httptest.NewRecorder()
	srv.handleQuota(rec, r)
	if rec.Code != http.StatusOK || calls != 1 {
		t.Fatalf("same-origin manual action = %d, calls = %d", rec.Code, calls)
	}
}
