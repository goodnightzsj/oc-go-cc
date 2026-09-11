package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/quota"
	"github.com/routatic/proxy/internal/storage"
)

// An opt-in real GUI/SQLite fixture for the browser acceptance script. All
// upstreams and credentials are synthetic; POST /api/proxy/stop ends the test.
// ROUTATIC_BROWSER_EMPTY=1 keeps the ledger empty for first-run acceptance.
func TestMultiPlatformBrowserServer(t *testing.T) {
	if os.Getenv("ROUTATIC_BROWSER_SMOKE") != "1" {
		t.Skip("set ROUTATIC_BROWSER_SMOKE=1 for the browser acceptance server")
	}
	// Inherited provider overrides must never replace a synthetic endpoint or key.
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "ROUTATIC_PROXY_") || strings.HasPrefix(name, "OC_GO_CC_") {
			t.Setenv(name, "")
		}
	}
	t.Setenv("HOME", t.TempDir())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docs":
			fmt.Fprint(w, `<table><tr><th>模型</th><th>输入</th><th>使用额度</th></tr><tr><td>Shared Model</td><td>$1</td><td>$60</td></tr></table>`)
		case "/go/usage":
			fmt.Fprintf(w, `{"plan":"go","rolling5h":{"usagePercent":25},"weekly":{"usagePercent":10},"monthly":{"usagePercent":50,"resetsAt":%q}}`, time.Now().UTC().Add(15*24*time.Hour).Format(time.RFC3339))
		case "/router/v1/key":
			if r.Header.Get("Authorization") == "Bearer synthetic-router-bad" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			fmt.Fprint(w, `{"data":{"limit":10,"limit_remaining":8,"limit_reset":"monthly","usage":2,"usage_daily":0,"usage_monthly":1,"byok_usage":3,"include_byok_in_limit":false,"is_free_tier":false}}`)
		case "/router/v1/credits":
			fmt.Fprint(w, `{"data":{"total_credits":1,"total_usage":2}}`)
		case "/command/alpha/billing/credits":
			fmt.Fprint(w, `{"credits":{"freeCredits":0,"monthlyCredits":60,"purchasedCredits":2},"windowLimits":{"limited":true,"fiveHour":{"used":3,"cap":14,"exceeded":false,"resetAt":0},"weekly":{"used":10,"cap":35,"exceeded":false,"resetAt":1910000000000}}}`)
		case "/command/alpha/billing/subscriptions":
			fmt.Fprint(w, `{"success":true,"data":{"planId":"individual-goat","status":"active","currentPeriodStart":"2026-09-10T07:35:57.000Z","currentPeriodEnd":"2026-10-10T07:35:57.000Z","cancelAtPeriodEnd":false}}`)
		case "/command/alpha/usage/summary":
			fmt.Fprint(w, `{"totalCount":2,"completedCount":1,"failedCount":1,"totalTokensIn":100,"totalTokensOut":20,"totalTokens":120,"totalCredits":10,"periodBasis":"billing-period"}`)
		case "/v1/messages":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"synthetic-message","type":"message","role":"assistant","content":[{"type":"text","text":"synthetic browser fixture"}],"model":"shared-model","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
		default:
			http.Error(w, "unexpected synthetic upstream endpoint", http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)
	_, upstreamPort, err := net.SplitHostPort(upstream.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(upstreamPort)
	if err != nil {
		t.Fatal(err)
	}
	providers := []string{"opencode-go", "opencode-zen", "aws-bedrock", "openrouter", "commandcode"}
	models := make(map[string]any)
	for _, provider := range providers {
		models[provider] = map[string]any{"provider": provider, "model_id": "shared-model", "max_tokens": 128}
	}
	raw, err := json.Marshal(map[string]any{
		"host": "127.0.0.1", "port": port, "catalog": map[string]any{"enabled": false},
		"models": map[string]any{"default": models["opencode-go"]}, "model_overrides": models,
		"fallbacks": map[string]any{"default": []any{models["opencode-zen"], models["aws-bedrock"], models["openrouter"], models["commandcode"]}},
		// Explicit stream timers let the browser verify sibling preservation;
		// omitted timers legitimately inherit a changed timeout_ms on reload.
		"opencode_go":  map[string]any{"base_url": upstream.URL + "/go/chat/completions", "api_key": "synthetic-go-key", "stream_timeout_ms": 310000},
		"opencode_zen": map[string]any{"base_url": upstream.URL + "/zen/chat/completions", "api_key": "synthetic-zen-key", "stream_timeout_ms": 310000},
		"aws_bedrock":  map[string]any{"base_url": upstream.URL + "/bedrock/chat/completions", "api_key": "synthetic-bedrock-key"},
		"openrouter":   map[string]any{"base_url": upstream.URL + "/router/v1/chat/completions", "api_keys": []string{"synthetic-router-good", "synthetic-router-bad"}, "management_api_key": "synthetic-management-key"},
		"commandcode":  map[string]any{"base_url": upstream.URL + "/command/provider/v1/chat/completions", "anthropic_base_url": upstream.URL + "/command/provider/v1/messages", "api_key": "synthetic-command-key", "stream_timeout_ms": 310000, "streaming_timeout_ms": 320000},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv, _ := configTestServer(t, string(raw))
	// The browser exercises opt-in and POST through the real GUI handler; this
	// synthetic collaborator prevents an AWS identity lookup or billable request.
	srv.fetchBedrockBilling = func(_ context.Context, billing config.AWSBillingConfig) (*quota.BedrockBilling, error) {
		if !billing.Enabled || billing.Profile != "browser-synthetic" || billing.LinkedAccountID != "123456789012" {
			return nil, fmt.Errorf("synthetic billing fixture requires its explicit identity and account")
		}
		end := time.Now().UTC().Truncate(24 * time.Hour)
		total := -1.25
		return &quota.BedrockBilling{
			LinkedAccountID: billing.LinkedAccountID, StartDate: end.AddDate(0, 0, -30).Format(time.DateOnly), EndDate: end.Format(time.DateOnly),
			Metric: "UnblendedCost", Services: []string{"Amazon Bedrock", "Amazon Bedrock Mantle"}, Currency: "EUR", TotalCost: &total, Estimated: true,
			Daily: []quota.BedrockDailyCost{{Date: end.AddDate(0, 0, -2).Format(time.DateOnly), Cost: -2.5}, {Date: end.AddDate(0, 0, -1).Format(time.DateOnly), Cost: 1.25, Estimated: true}},
		}, nil
	}
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "browser.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stamp := time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339Nano)
	for i, provider := range providers {
		if os.Getenv("ROUTATIC_BROWSER_EMPTY") == "1" {
			break
		}
		for known := 0; known <= 1; known++ {
			var cost, success any
			if known == 1 {
				cost, success = float64(i+1)/10, 1
			}
			if _, err := db.DB().Exec(`INSERT INTO requests
				(id, provider, model, scenario, start_time, input_tokens, output_tokens, cache_read_tokens,
				 cache_creation_tokens, details_known, success, duration_ms, cost_usd, cost_source)
				VALUES (?, ?, 'shared-model', 'default', ?, ?, 2, 3, 1, ?, ?, ?, ?, 'estimated')`,
				fmt.Sprintf("synthetic-%s-%d", provider, known), provider, stamp, (i+1)*10, known, success, known*(i+1)*100, cost); err != nil {
				t.Fatal(err)
			}
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	guiPort := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROUTATIC_PROXY_GUI_PORT", strconv.Itoa(guiPort))
	srv.storage = db
	srv.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	srv.modelLimitsURL = []string{upstream.URL + "/docs"}
	srv.SetProxyRunning(true)
	finished := make(chan struct{})
	var once sync.Once
	srv.stopProxy = func() error { once.Do(func() { close(finished) }); return nil }
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	address, err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	fmt.Printf("BROWSER_SMOKE_URL=%s\n", address)
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("browser acceptance server timed out without completion")
	}
}
