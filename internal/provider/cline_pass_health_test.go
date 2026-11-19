package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/quota"
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

// The first pass is synchronous and bounded: by the time StartHealthLoop
// returns, the gate already holds verdicts, so a request arriving right after
// boot cannot be handed a probe-dead key.
func TestStartHealthLoopProbesBeforeServing(t *testing.T) {
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.Header.Get("Authorization"), healthDeadKey) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"no plan history found for user"}`)
			return
		}
		_, _ = io.WriteString(w, `{"success":true,"data":{"limits":[{"type":"five_hour","percentUsed":10}]}}`)
	})
	cfg := p.atomic.Get()
	cfg.ClinePass.APIKey = ""
	cfg.ClinePass.APIKeys = []string{healthDeadKey, healthLiveKey}

	p.StartHealthLoop(t.Context())
	if !p.health.Excluded(healthDeadKey) {
		t.Fatal("first pass must complete before StartHealthLoop returns")
	}
	if p.health.Excluded(healthLiveKey) {
		t.Fatal("a live key must not be excluded by the first pass")
	}
}

// The fallback handler's "can this platform serve?" question must agree with
// the keys Reserve would actually pick: probe-dead keys are not candidates, and
// when the remaining keys are all cooling the platform is not usable even
// though the dead ones are idle.
func TestKeyAvailabilityConsultsHealthGate(t *testing.T) {
	p, _ := clinePassTestProvider(t, func(http.ResponseWriter, *http.Request) {})
	const liveOne, liveTwo, dead = "synthetic-live-one", "synthetic-live-two", "synthetic-dead-three"
	cfg := p.atomic.Get()
	cfg.ClinePass.APIKey = ""
	cfg.ClinePass.APIKeys = []string{liveOne, liveTwo, dead}
	p.health.Record(dead, &quota.HTTPStatusError{Status: 404, Err: errors.New("HTTP 404: no plan history found for user")})
	for _, key := range []string{liveOne, liveTwo} {
		p.keys.Reserve([]string{key}).Settle(&client.APIError{StatusCode: 401})
	}
	keys := cfg.ProviderAPIKeys(client.ProviderClinePass)
	// Control: the raw pool still sees the idle dead key and claims a usable key.
	if !p.keys.HasUsable(keys) {
		t.Fatal("control: raw pool should report the dead key as usable")
	}
	if NewKeyAvailability(p.atomic).HasUsableKey(client.ProviderClinePass) {
		t.Fatal("gate-aware availability must not count probe-dead keys")
	}
}

// An auth rejection re-judges the key immediately, so a dead key leaves
// rotation in about a second instead of waiting for the next periodic pass.
// Repeated rejections are floored rather than probing on every one.
func TestAuthVerdictTriggersImmediateProbe(t *testing.T) {
	var (
		hits      int32
		probeDead atomic.Bool
	)
	key := "synthetic-credential-flip-0001"
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/me/plan/usage-limits" {
			t.Errorf("unexpected probe path %s", r.URL.Path)
			return
		}
		atomic.AddInt32(&hits, 1)
		if probeDead.Load() {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"no plan history found for user"}`)
			return
		}
		_, _ = io.WriteString(w, `{"success":true,"data":{"limits":[{"type":"five_hour","percentUsed":10}]}}`)
	})
	cfg := p.atomic.Get()
	cfg.ClinePass.APIKey = ""
	cfg.ClinePass.APIKeys = []string{key}

	p.StartHealthLoop(t.Context())
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("first pass probes=%d want 1", got)
	}
	if p.health.Excluded(key) {
		t.Fatal("key started healthy")
	}

	probeDead.Store(true)
	p.keys.Reserve([]string{key}).Settle(&client.APIError{StatusCode: 403})
	deadline := time.Now().Add(3 * time.Second)
	for !p.health.Excluded(key) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !p.health.Excluded(key) {
		t.Fatal("an auth rejection must trigger an immediate probe that excludes the key")
	}
	before := atomic.LoadInt32(&hits)

	// Second rejection inside the floor window must not spend another probe.
	p.keys.Reserve([]string{key}).Settle(&client.APIError{StatusCode: 403})
	time.Sleep(150 * time.Millisecond)
	if after := atomic.LoadInt32(&hits); after != before {
		t.Fatalf("probes=%d want %d: repeated rejections must respect the floor", after, before)
	}
}

// A hot-reloaded base_url names a different account authority: verdicts from
// the old endpoint must not survive the switch.
func TestHealthScopeResetsVerdictsOnEndpointChange(t *testing.T) {
	key := "synthetic-credential-mirror-0002"
	rejecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"no plan history found for user"}`)
	}))
	t.Cleanup(rejecting.Close)
	accepting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"data":{"limits":[{"type":"five_hour","percentUsed":10}]}}`)
	}))
	t.Cleanup(accepting.Close)

	p, _ := clinePassTestProvider(t, func(http.ResponseWriter, *http.Request) {})
	cfg := p.atomic.Get()
	cfg.ClinePass.APIKey = ""
	cfg.ClinePass.APIKeys = []string{key}
	cfg.ClinePass.BaseURL = rejecting.URL + "/api/v1/chat/completions"

	p.probeKeyHealth(t.Context())
	if !p.health.Excluded(key) {
		t.Fatal("the rejecting endpoint must exclude the key")
	}

	cfg.ClinePass.BaseURL = accepting.URL + "/api/v1/chat/completions"
	p.probeKeyHealth(t.Context())
	if p.health.Excluded(key) {
		t.Fatal("a verdict recorded against the old endpoint must not survive the switch")
	}
}
