package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestQuotaPlatformCapabilitiesDoNotSendGoKeys(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a platform without a quota API sent a credential to Go")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	srv := quotaTestServer(t, upstream.URL, "synthetic-go-key")
	cfg := *srv.atomicCfg.Get()
	cfg.OpenCodeZen.APIKey = "synthetic-zen-key"
	cfg.AWSBedrock.APIKey = "synthetic-bedrock-key"
	cfg.CommandCode.APIKey = "synthetic-commandcode-key"
	srv.atomicCfg.ApplyLoaded(&cfg)
	for provider, reason := range map[string]string{
		"opencode-zen": "no_public_account_api", "commandcode": "no_public_account_api",
		"aws-bedrock": "aws_billing_disabled",
	} {
		t.Run(provider, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.handleQuota(rec, httptest.NewRequest(http.MethodGet, "/api/quota?provider="+provider, nil))
			var body struct {
				Provider, Status, Source, Reason string
				Accounts                         []json.RawMessage
				Links                            []map[string]string
			}
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
				t.Fatalf("capability response = %d: %s", rec.Code, rec.Body.String())
			}
			status, source := "unavailable", "none"
			if provider == "aws-bedrock" {
				status, source = "not_configured", "official_api"
			}
			if body.Provider != provider || body.Status != status || body.Source != source || body.Reason != reason || len(body.Accounts) != 0 || len(body.Links) == 0 {
				t.Fatalf("platform capability misrepresented: %+v", body)
			}
			if strings.Contains(rec.Body.String(), "synthetic-") {
				t.Fatal("capability response exposed credentials")
			}
		})
	}
	rec := httptest.NewRecorder()
	srv.handleQuota(rec, httptest.NewRequest(http.MethodGet, "/api/quota?provider=unknown", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatal("unknown quota platform did not fail explicitly")
	}
}

func TestOpenRouterQuotaScopeAndCache(t *testing.T) {
	var keyHits, creditsHits, goHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/key":
			keyHits.Add(1)
			switch r.Header.Get("Authorization") {
			case "Bearer synthetic-or-good":
				fmt.Fprint(w, `{"data":{"limit":10,"limit_remaining":8,"limit_reset":"monthly","usage":2,"usage_daily":0,"byok_usage":7,"include_byok_in_limit":false}}`)
			case "Bearer synthetic-or-bad":
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, r.Header.Get("Authorization"))
			default:
				t.Error("wrong key sent to the inference-key quota endpoint")
				w.WriteHeader(http.StatusForbidden)
			}
		case "/api/v1/credits":
			creditsHits.Add(1)
			if r.Header.Get("Authorization") != "Bearer synthetic-management" {
				t.Error("inference key or other platform key used for account credits")
			}
			fmt.Fprint(w, `{"data":{"total_credits":1,"total_usage":2}}`)
		case "/go/usage":
			goHits.Add(1)
			fmt.Fprint(w, `{"monthly":{"usagePercent":25}}`)
		default:
			t.Errorf("unexpected upstream path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	raw := fmt.Sprintf(`{"host":"127.0.0.1","port":3456,"opencode_go":{"api_key":"synthetic-go","base_url":%q},"openrouter":{"base_url":%q,"api_keys":["synthetic-or-good","synthetic-or-bad","synthetic-or-good"],"management_api_key":"synthetic-management"}}`, upstream.URL+"/go/chat/completions", upstream.URL+"/api/v1/chat/completions")
	srv, _ := configTestServer(t, raw)
	srv.modelLimitsURL = []string{"http://127.0.0.1:1/no"}
	call := func(query string) map[string]any {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.handleQuota(rec, httptest.NewRequest(http.MethodGet, "/api/quota?"+query, nil))
		var result map[string]any
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil {
			t.Fatalf("quota %s = %d: %s", query, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "synthetic-") || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("quota response exposed credentials or is browser-cacheable")
		}
		return result
	}
	result := call("provider=openrouter")
	if result["provider"] != "openrouter" || result["status"] != "partial" || result["source"] != "official_api" || result["credits_status"] != "available" || result["currency"] != "USD" {
		t.Fatalf("wrong OpenRouter provenance/status: %v", result)
	}
	accounts, ok := result["accounts"].([]any)
	if !ok || len(accounts) != 2 {
		t.Fatalf("per-key quota was not deduplicated: %v", result["accounts"])
	}
	first := accounts[0].(map[string]any)
	second := accounts[1].(map[string]any)
	info := first["openrouter"].(map[string]any)
	if info["usage"] != float64(2) || info["byok_usage"] != float64(7) || info["usage_monthly"] != nil || info["usage_daily"] != float64(0) || second["error"] == nil || second["openrouter"] != nil {
		t.Fatal("partial quota confused unknown, BYOK, zero or failed key data")
	}
	credits := result["credits"].(map[string]any)
	if credits["total_credits"].(float64)-credits["total_usage"].(float64) != -1 {
		t.Fatal("negative account balance was lost")
	}
	call("provider=opencode-go")
	if !call("provider=openrouter")["cached"].(bool) || keyHits.Load() != 2 || creditsHits.Load() != 1 || goHits.Load() != 1 {
		t.Fatal("switching platforms bypassed the per-platform cache")
	}
	call("provider=openrouter&refresh=1")
	if keyHits.Load() != 4 || creditsHits.Load() != 2 {
		t.Fatal("explicit refresh did not refresh the selected platform")
	}
	var patch map[string]json.RawMessage
	json.Unmarshal([]byte(`{"openrouter":{"management_api_key":""}}`), &patch)
	if _, err := srv.updateProxyConfig(patch, true); err != nil {
		t.Fatal(err)
	}
	result = call("provider=openrouter")
	if result["credits_status"] != "not_configured" || result["credits"] != nil || result["cached"] != false || creditsHits.Load() != 2 {
		t.Fatal("management-key removal reused stale balance or sent an inference key")
	}
}

func TestOpenRouterManagementKeySettingsRoundTrip(t *testing.T) {
	t.Setenv("GUI_TEST_MANAGEMENT_KEY", "synthetic-management")
	srv, path := configTestServer(t, `{"api_key":"synthetic-global","openrouter":{"api_key":"synthetic-inference","management_api_key":"${GUI_TEST_MANAGEMENT_KEY}"}}`)
	for _, handler := range []http.HandlerFunc{srv.handleProxyConfig, srv.handleConfigExport} {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "synthetic-") {
			t.Fatal("settings read leaked a credential")
		}
		var result config.Config
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		encoded, _ := json.Marshal(result.OpenRouter)
		json.Unmarshal(encoded, &fields)
		if fields["management_api_key"] != keyMask {
			t.Fatal("management key is missing from masked settings")
		}
	}
	rec := httptest.NewRecorder()
	patch := `{"openrouter":{"timeout_ms":1234,"management_api_key":"` + keyMask + `"}}`
	srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(patch)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("partial save = %d: %s", rec.Code, rec.Body.String())
	}
	saved, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(saved), "${GUI_TEST_MANAGEMENT_KEY}") || strings.Contains(string(saved), keyMask) || strings.Contains(string(saved), "synthetic-management") {
		t.Fatal("partial save changed the management key placeholder")
	}
}

func TestOpenRouterQuotaDoesNotProbeGlobalKeys(t *testing.T) {
	for _, managementOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("management-only=%t", managementOnly), func(t *testing.T) {
			var keyHits, creditsHits atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/credits" {
					creditsHits.Add(1)
					if r.Header.Get("Authorization") != "Bearer synthetic-management" {
						t.Error("account credits used an unrelated credential")
					}
					fmt.Fprint(w, `{"data":{"total_credits":1,"total_usage":0}}`)
					return
				}
				keyHits.Add(1)
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer upstream.Close()
			managementKey := ""
			if managementOnly {
				managementKey = "synthetic-management"
			}
			raw := fmt.Sprintf(`{"api_key":"synthetic-global-go","openrouter":{"base_url":%q,"management_api_key":%q}}`,
				upstream.URL+"/api/v1/chat/completions", managementKey)
			srv, _ := configTestServer(t, raw)
			rec := httptest.NewRecorder()
			srv.handleQuota(rec, httptest.NewRequest(http.MethodGet, "/api/quota?provider=openrouter", nil))
			var response quotaResponse
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &response) != nil {
				t.Fatalf("quota = HTTP %d: %s", rec.Code, rec.Body.String())
			}
			if keyHits.Load() != 0 || len(response.Accounts) != 0 {
				t.Error("browsing an unconfigured platform sent a global inference key upstream")
			}
			if managementOnly {
				if creditsHits.Load() != 1 || response.Credits == nil || response.Status != "available" {
					t.Fatal("explicit management-key lookup was lost")
				}
			} else if creditsHits.Load() != 0 || response.Status != "not_configured" {
				t.Fatal("unconfigured platform was treated as authorized")
			}
		})
	}
}
