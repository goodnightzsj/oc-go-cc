package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/metrics"
)

func quotaTestServer(t *testing.T, upstream string, keys ...string) *Server {
	t.Helper()
	cfg := &config.Config{Host: "127.0.0.1", Port: 3456}
	cfg.OpenCodeGo.BaseURL = upstream + "/chat/completions"
	cfg.OpenCodeGo.APIKeys = keys
	// The allowance-table fetch must never leave the test process; point it at
	// a dead local port so ensureModelLimits fails fast and stays empty.
	return &Server{atomicCfg: config.NewAtomicConfig(cfg, ""), modelLimitsURL: []string{"http://127.0.0.1:1/no"}}
}

// docsFixture is a minimal Go docs allowance table in the zh layout.
const docsFixture = `<html><body><table><tr><th>模型</th><th>输入</th><th>使用额度</th></tr>
<tr><td>GLM-5.2</td><td>$1.40</td><td>$60</td></tr>
<tr><td>Grok 4.6 (≤ 200K tokens)</td><td>$2.00</td><td>$15</td></tr>
<tr><td>DeepSeek V4 Flash (Off-Peak)</td><td>$0.22</td><td>$30</td></tr>
<tr><td>Omen &amp; Alpha</td><td>$0.20</td><td>$100</td></tr>
<tr><td>No Allowance</td><td>$1.00</td><td>-</td></tr></table></body></html>`

func getQuota(t *testing.T, srv *Server, query string) quotaResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.handleQuota(rec, httptest.NewRequest(http.MethodGet, "/api/quota"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp quotaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	return resp
}

func TestHandleQuotaMasksKeysAndCaches(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/usage" {
			t.Errorf("upstream path = %q, want /usage", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"plan":"go","rolling5h":{"usagePercent":25,"usageDollars":3,"limitDollars":12}}`))
	}))
	defer upstream.Close()

	srv := quotaTestServer(t, upstream.URL, "sk-secret-key-1234")
	resp := getQuota(t, srv, "")
	if len(resp.Accounts) != 1 {
		t.Fatalf("accounts = %d, want 1", len(resp.Accounts))
	}
	account := resp.Accounts[0]
	if account.Error != "" {
		t.Fatalf("unexpected error: %s", account.Error)
	}
	if account.KeyHint != "••••1234" {
		t.Errorf("key hint = %q, want ••••1234", account.KeyHint)
	}
	if account.Report == nil || account.Report.Rolling5h == nil || account.Report.Rolling5h.UsedPercent != 25 {
		t.Fatalf("unexpected report: %+v", account.Report)
	}
	if resp.Cached {
		t.Error("first response must not be marked cached")
	}

	// A second call inside the TTL is served from cache, so the undocumented
	// upstream endpoint is polled once.
	if cached := getQuota(t, srv, ""); !cached.Cached {
		t.Error("second response should be cached")
	}
	if hits.Load() != 1 {
		t.Errorf("upstream hits = %d, want 1", hits.Load())
	}

	// refresh=1 bypasses the cache.
	if forced := getQuota(t, srv, "?refresh=1"); forced.Cached {
		t.Error("refresh=1 must bypass the cache")
	}
	if hits.Load() != 2 {
		t.Errorf("upstream hits after forced refresh = %d, want 2", hits.Load())
	}
}

func TestHandleQuotaNeverLeaksTheKey(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"rolling5h":{"usagePercent":1}}`))
	}))
	defer upstream.Close()

	srv := quotaTestServer(t, upstream.URL, "sk-do-not-leak-abcd")
	rec := httptest.NewRecorder()
	srv.handleQuota(rec, httptest.NewRequest(http.MethodGet, "/api/quota", nil))
	if strings.Contains(rec.Body.String(), "sk-do-not-leak-abcd") {
		t.Fatalf("response body leaked the API key: %s", rec.Body.String())
	}
}

func TestHandleQuotaReportsPerKeyErrors(t *testing.T) {
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer denied.Close()

	srv := quotaTestServer(t, denied.URL, "sk-aaaa", "sk-bbbb")
	resp := getQuota(t, srv, "")
	if len(resp.Accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(resp.Accounts))
	}
	for i, account := range resp.Accounts {
		if account.Error == "" {
			t.Errorf("account %d: expected an error for HTTP 401", i)
		}
		if account.Report != nil {
			t.Errorf("account %d: report must be empty when the fetch failed", i)
		}
	}
}

func TestHandleQuotaWithoutKeysOrDerivableEndpoint(t *testing.T) {
	cfg := &config.Config{Host: "127.0.0.1", Port: 3456}
	srv := &Server{atomicCfg: config.NewAtomicConfig(cfg, "")}
	resp := getQuota(t, srv, "")
	if len(resp.Accounts) != 0 {
		t.Errorf("accounts = %d, want 0 without a configured key", len(resp.Accounts))
	}
	if resp.Endpoint == "" {
		t.Error("the default endpoint should still be reported so the UI can show it")
	}

	cfg2 := &config.Config{Host: "127.0.0.1", Port: 3456}
	cfg2.OpenCodeGo.BaseURL = "https://mirror.example/not-an-api"
	cfg2.OpenCodeGo.APIKey = "sk-xyz"
	bad := &Server{atomicCfg: config.NewAtomicConfig(cfg2, "")}
	// A base URL the usage path cannot be derived from must fail closed rather
	// than send the key to the public host.
	if resp := getQuota(t, bad, ""); resp.Error == "" || len(resp.Accounts) != 0 {
		t.Errorf("expected a fail-closed error, got %+v", resp)
	}
}

func TestHandleQuotaAttachesModelLimits(t *testing.T) {
	var docsHits, usageHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/docs" {
			docsHits.Add(1)
			_, _ = w.Write([]byte(docsFixture))
			return
		}
		usageHits.Add(1)
		_, _ = w.Write([]byte(`{"rolling5h":{"usagePercent":1}}`))
	}))
	defer upstream.Close()

	srv := quotaTestServer(t, upstream.URL, "sk-key")
	srv.modelLimitsURL = []string{upstream.URL + "/docs"}
	srv.met = metrics.New()
	srv.met.RecordSuccess("glm-5.2", time.Millisecond)
	srv.met.RecordSuccess("deepseek-v4-flash", time.Millisecond)

	resp := getQuota(t, srv, "")
	if resp.ModelLimits == nil || len(resp.ModelLimits.Models) != 4 {
		t.Fatalf("model_limits = %+v, want 4 models", resp.ModelLimits)
	}
	first := resp.ModelLimits.Models[0]
	if first.Model != "GLM-5.2" || first.AllowanceUSD != 60 {
		t.Errorf("first model = %+v, want GLM-5.2 $60", first)
	}
	if resp.ModelLimits.Models[3].Model != "Omen & Alpha" {
		t.Errorf("HTML entities must be unescaped, got %q", resp.ModelLimits.Models[3].Model)
	}
	if resp.ModelLimits.URL != upstream.URL+"/docs" {
		t.Errorf("limits url = %q", resp.ModelLimits.URL)
	}
	// The two recorded models must be flagged used, including the variant
	// match ("deepseek-v4-flash" ↔ "DeepSeek V4 Flash (Off-Peak)").
	if !resp.ModelLimits.Models[0].Used || !resp.ModelLimits.Models[2].Used {
		t.Errorf("used flags = %+v, want GLM-5.2 and DeepSeek flagged", []bool{resp.ModelLimits.Models[0].Used, resp.ModelLimits.Models[2].Used})
	}
	if resp.ModelLimits.Models[1].Used || resp.ModelLimits.Models[3].Used {
		t.Error("unrecorded models must not be flagged used")
	}
	if docsHits.Load() != 1 {
		t.Errorf("docs hits = %d, want 1", docsHits.Load())
	}

	// A cached quota response carries the same snapshot; the docs page is not
	// re-fetched inside the TTL.
	_ = getQuota(t, srv, "")
	if docsHits.Load() != 1 {
		t.Errorf("docs hits after cached response = %d, want 1", docsHits.Load())
	}

	// Stale (>24h) limits refresh on the next uncached request.
	srv.limitsMu.Lock()
	srv.modelLimits.FetchedAt = time.Now().Add(-25 * time.Hour)
	srv.limitsMu.Unlock()
	_ = getQuota(t, srv, "?refresh=1")
	if docsHits.Load() != 2 {
		t.Errorf("docs hits after stale refresh = %d, want 2", docsHits.Load())
	}
	if usageHits.Load() != 2 {
		t.Errorf("usage hits = %d, want 2 (cached + forced)", usageHits.Load())
	}
}

func TestGoQuotaKeysPrecedenceAndDedup(t *testing.T) {
	if got := goQuotaKeys([]string{"sk-a", " sk-a ", "", "sk-b"}, []string{"sk-global"}); len(got) != 2 || got[0] != "sk-a" || got[1] != "sk-b" {
		t.Errorf("provider keys should win and dedup, got %v", got)
	}
	if got := goQuotaKeys(nil, []string{"sk-global"}); len(got) != 1 || got[0] != "sk-global" {
		t.Errorf("global keys should be the fallback, got %v", got)
	}
	if got := goQuotaKeys(nil, nil); len(got) != 0 {
		t.Errorf("no keys should yield an empty pool, got %v", got)
	}
}
