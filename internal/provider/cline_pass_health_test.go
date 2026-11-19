package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

const (
	healthDeadKey = "synthetic-credential-dead-4040"
	healthLiveKey = "synthetic-credential-live-abcd"
)

// The subscription probe gates key rotation: an account whose probe answers
// 404 - the upstream's "no plan history found for user" - never takes an
// inference request, even when it sits first in fill-first order, and a later
// successful probe returns it.
func TestKeyHealthGateExcludesProbeDeadKeysFromRequests(t *testing.T) {
	var mu sync.Mutex
	var inferenceBearers []string
	probeDead := true
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/users/me/plan/usage-limits":
			if probeDead && strings.HasSuffix(r.Header.Get("Authorization"), healthDeadKey) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"error":"no plan history found for user"}`)
				return
			}
			_, _ = io.WriteString(w, `{"success":true,"data":{"limits":[{"type":"five_hour","percentUsed":10}]}}`)
		default:
			mu.Lock()
			inferenceBearers = append(inferenceBearers, r.Header.Get("Authorization"))
			mu.Unlock()
			_, _ = io.WriteString(w, "data: {\"id\":\"gen-1\",\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"id\":\"gen-1\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		}
	})
	// Replace the harness's single key with a pool whose dead key sits first,
	// so fill-first alone would hand every request to a 403.
	cfg := p.atomic.Get()
	cfg.ClinePass.APIKey = ""
	cfg.ClinePass.APIKeys = []string{healthDeadKey, healthLiveKey}

	// Before any probe, fill-first serves the dead first-configured key.
	p.probeKeyHealth(context.Background())
	if got := p.health.Filter(cfg.ClinePass.EffectiveAPIKeys()); len(got) != 1 || got[0] != healthLiveKey {
		t.Fatalf("filter after probe = %v, want only the live key", got)
	}
	if !p.health.Excluded(healthDeadKey) || p.health.Excluded(healthLiveKey) {
		t.Fatal("verdicts must mark exactly the dead key excluded")
	}

	// The gated request reaches the live credential, not the first-configured
	// dead one; without the gate fill-first would have sent it to 403.
	mu.Lock()
	inferenceBearers = nil
	mu.Unlock()
	if _, err := p.Execute(context.Background(), clinePassRequest(), config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/glm-5.3"}); err != nil {
		t.Fatalf("gated request failed: %v", err)
	}
	mu.Lock()
	bearers := append([]string(nil), inferenceBearers...)
	mu.Unlock()
	if len(bearers) != 1 || !strings.HasSuffix(bearers[0], healthLiveKey) {
		t.Fatalf("inference bearers = %v, want exactly the live key", bearers)
	}

	// Recovery: once the probe sees a subscription again, the key returns to
	// rotation and fill-first serves it again.
	probeDead = false
	p.probeKeyHealth(context.Background())
	if p.health.Excluded(healthDeadKey) {
		t.Fatal("a successful probe must return the key to rotation")
	}
	mu.Lock()
	inferenceBearers = nil
	mu.Unlock()
	if _, err := p.Execute(context.Background(), clinePassRequest(), config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/glm-5.3"}); err != nil {
		t.Fatalf("request after recovery failed: %v", err)
	}
	mu.Lock()
	bearers = append([]string(nil), inferenceBearers...)
	mu.Unlock()
	if len(bearers) != 1 || !strings.HasSuffix(bearers[0], healthDeadKey) {
		t.Fatalf("after recovery bearers = %v, want the recovered first key", bearers)
	}
}
