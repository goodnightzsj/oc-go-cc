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

func TestRound2ClineDistinctKeysWithSameHintReceiveOwnLimits(t *testing.T) {
	var planCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/users/me/plan/usage-limits":
			_, _ = fmt.Fprint(w, `{"success":true,"data":{"limits":[{"type":"five_hour","percentUsed":50}]}}`)
		case "/api/v1/users/me/plan":
			planCalls.Add(1)
			limit := 1000000000
			if r.Header.Get("Authorization") == "Bearer synthetic-b-1234" {
				limit = 2000000000
			}
			_, _ = fmt.Fprintf(w, `{"success":true,"data":{"plan":{"entitlements":{"cline_pass":{"enabled":true,"inferenceCapThreshold":{"last5HoursUsageCostUSDPerUser":%d}}}}}}`, limit)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	s := quotaTestServer(t, upstream.URL)
	cfg := *s.atomicCfg.Get()
	cfg.ClinePass.BaseURL = upstream.URL + "/api/v1/chat/completions"
	cfg.ClinePass.APIKeys = []string{"synthetic-a-1234", "synthetic-b-1234", "synthetic-a-1234"}
	s.atomicCfg.ApplyLoaded(&cfg)
	w := httptest.NewRecorder()
	s.handleQuota(w, httptest.NewRequest(http.MethodGet, "/api/quota?provider=cline-pass", nil))
	if strings.Contains(w.Body.String(), "synthetic-") {
		t.Fatal("response leaked credential")
	}
	var result quotaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Accounts) != 2 || result.Accounts[0].ClinePass == nil || result.Accounts[1].ClinePass == nil {
		t.Fatal("both synthetic accounts must have successful usage reports")
	}
	first := result.Accounts[0].ClinePass.Windows[0].LimitUSD
	second := result.Accounts[1].ClinePass.Windows[0].LimitUSD
	if planCalls.Load() != 2 || first != 10 || second != 20 {
		t.Fatalf("display hint merged distinct accounts: planCalls=%d ceilings=[%v %v]", planCalls.Load(), first, second)
	}
}
