package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
)

func TestChannelPinProbePrefersRequestedAndConfiguredModels(t *testing.T) {
	p := NewClinePassProvider(config.NewAtomicConfig(&config.Config{}, ""), nil)
	cfg := &config.Config{ModelOverrides: map[string]config.ModelConfig{
		"alias": {Provider: p.Name(), ModelID: "configured"},
		"other": {Provider: "workbuddy", ModelID: "not-cline"},
	}}
	p.rememberProbeModel(cfg, "first")
	p.rememberProbeModel(cfg, "current")
	want := []string{"current", "first", "configured"}
	if got := p.clinePassProbeModels(t.Context(), cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("probe scope = %v want %v", got, want)
	}
	for _, id := range []string{"third", "fourth", "current"} {
		p.rememberProbeModel(cfg, id)
	}
	want = []string{"current", "fourth", "third"}
	if got := p.clinePassProbeModels(t.Context(), cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("recent scope not bounded/deduplicated: %v", got)
	}
}

func TestChannelPinProbeRejectsMalformedLateRouting(t *testing.T) {
	for _, baseline := range []bool{false, true} {
		t.Run(fmt.Sprintf("baseline=%t", baseline), func(t *testing.T) {
			srv := probeUpstream(t, []string{"synthetic"}, func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				target := plannerPin(request)
				if target == "" {
					target = directPin(request)
				}
				body := routingFrame("minimax", "minimax", "nebius")
				if target != "" {
					body = routingFrame(target)
				}
				if baseline || target != "" {
					bad := "data: {\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":123}}}}\n\n"
					body = strings.Replace(body, "data: [DONE]", bad+"data: [DONE]", 1)
				}
				_, _ = io.WriteString(w, body)
			})
			got := probeVerdict(t, probeConfig(t, srv))
			if strings.Contains(got, "verdict=matched") || strings.Contains(got, "verdict=ignored") || strings.Count(got, "verdict=unknown") != 2 {
				t.Fatalf("invalid final routing must not reuse partial evidence: %s", got)
			}
		})
	}
}

func TestChannelPinProbeRecentModelsFollowConfigurationScope(t *testing.T) {
	old := &config.Config{ClinePass: config.ClinePassConfig{BaseURL: "https://old.invalid/chat/completions", APIKeys: []string{"synthetic-old"}}}
	p := NewClinePassProvider(config.NewAtomicConfig(old, ""), nil)
	p.rememberProbeModel(old, "old-model")
	for _, changed := range []string{"endpoint", "keys"} {
		t.Run(changed, func(t *testing.T) {
			next := *old
			next.Models = map[string]config.ModelConfig{"default": {Provider: p.Name(), ModelID: "current-model"}}
			if changed == "endpoint" {
				next.ClinePass.BaseURL = "https://new.invalid/chat/completions"
			} else {
				next.ClinePass.APIKeys = []string{"synthetic-new"}
			}
			if got := p.clinePassProbeModels(t.Context(), &next); !reflect.DeepEqual(got, []string{"current-model"}) {
				t.Fatalf("stale probe models after %s change: %v", changed, got)
			}
		})
	}
}

func TestChannelPinProbeScopeAndFreshSessions(t *testing.T) {
	var mu sync.Mutex
	sessions := map[string]bool{}
	srv := probeUpstream(t, []string{"synthetic"}, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		id := r.Header.Get("X-Session-Id")
		if id == "" || sessions[id] {
			t.Error("probe reused a session")
		}
		sessions[id] = true
		_, _ = io.WriteString(w, routingFrame("minimax", "minimax", "nebius"))
	})
	cfg := probeConfig(t, srv)
	got := probeVerdict(t, cfg)
	for _, field := range []string{"endpoint=", "key_hint=", "checked_at=", "expires_at=", "enforcement_verified=false", "session_policy=fresh-per-request", "capabilities=none"} {
		if !strings.Contains(got, field) {
			t.Errorf("probe omitted %s: %s", field, got)
		}
	}
	if strings.Contains(got, cfg.ClinePass.APIKey) {
		t.Fatal("probe logged a credential")
	}
	if len(sessions) != 4 {
		t.Fatalf("probe requests=%d want 4", len(sessions))
	}
}

func TestChannelPinProbeUsesHealthyKeyWithoutChangingHealth(t *testing.T) {
	srv := probeUpstream(t, []string{"synthetic"}, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-healthy" {
			t.Error("probe ignored healthy-key selection")
		}
		_, _ = io.WriteString(w, routingFrame("minimax"))
	})
	cfg := probeConfig(t, srv)
	cfg.ClinePass.APIKey = ""
	cfg.ClinePass.APIKeys = []string{"synthetic-blocked", "synthetic-healthy"}
	p := NewClinePassProvider(config.NewAtomicConfig(cfg, ""), nil)
	p.keys.Reserve(cfg.ClinePass.APIKeys).Settle(&client.APIError{StatusCode: 401})
	blocked := p.keys.blockedUntil("synthetic-blocked")
	p.runChannelPinProbe(t.Context())
	if p.keys.blockedUntil("synthetic-blocked") != blocked {
		t.Fatal("probe changed request health")
	}
	for _, st := range p.keys.states {
		if st.inFlight != 0 {
			t.Fatal("probe leaked a reservation")
		}
	}
}

func TestChannelPinProbeRequiresChatTerminal(t *testing.T) {
	body := strings.ReplaceAll(routingFrame("private"), `,"finish_reason":"stop"`, "")
	if probeStreamFailure(body) == "" {
		t.Fatal("[DONE] without Chat completion is not successful evidence")
	}
	valid := strings.ReplaceAll(routingFrame("private"), `"gateway":{`, "\"gateway\":\ndata: {")
	if reason := probeStreamFailure(valid); reason != "" {
		t.Fatal(reason)
	}
	if got, _, _, err := routingSummary(valid); err != nil || got != "private" {
		t.Fatalf("multiline routing=%q err=%v", got, err)
	}
}

func TestChannelDiagnosticsScopedReadOnlySnapshot(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := probeUpstream(t, []string{"synthetic"}, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		target := plannerPin(body)
		if target == "" {
			target = directPin(body)
		}
		if target == "" {
			_, _ = io.WriteString(w, routingFrame("minimax", "minimax", "nebius"))
		} else {
			_, _ = io.WriteString(w, routingFrame(target))
		}
	})
	cfg := probeConfig(t, srv)
	cfg.ClinePass.BaseURL += "?private=synthetic-endpoint-secret"
	cfg.ClinePass.APIKey = "synthetic-credential-secret"
	cfg.Models = map[string]config.ModelConfig{"default": {Provider: "cline-pass", ModelID: "synthetic"}}
	a := config.NewAtomicConfig(cfg, "")
	p := NewClinePassProvider(a, nil)
	p.runChannelPinProbe(t.Context())
	result := p.ChannelDiagnostics()
	if !result.Enabled || result.EnforcementVerified || len(result.Results) != 2 {
		t.Fatalf("snapshot=%+v", result)
	}
	for _, row := range result.Results {
		if row.Verdict != "matched" || row.ExpiresAt.Sub(row.CheckedAt) != probeInterval || row.Endpoint != srv.URL+"/api/v1/chat/completions" || row.KeyHint == "" {
			t.Fatalf("result=%+v", row)
		}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{cfg.ClinePass.APIKey, "synthetic-endpoint-secret", "detail"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("diagnostics exposed credentials, query or raw detail")
		}
	}
	result.Results[0].Model = "mutated reader"
	for i := 0; i < 3; i++ {
		if p.ChannelDiagnostics().Results[0].Model == "mutated reader" {
			t.Fatal("snapshot shares mutable owner state")
		}
	}
	mu.Lock()
	if calls != 4 {
		t.Errorf("reads sent paid requests: %d", calls)
	}
	mu.Unlock()
	for _, change := range []string{"endpoint", "key", "disabled"} {
		next := *cfg
		switch change {
		case "endpoint":
			next.ClinePass.BaseURL = "https://new.invalid/chat/completions"
		case "key":
			next.ClinePass.APIKey = "synthetic-new-key"
		case "disabled":
			next.ClinePass.ChannelProbeEnabled = false
		}
		a.ApplyLoaded(&next)
		if got := p.ChannelDiagnostics(); len(got.Results) != 0 || got.EnforcementVerified || got.Enabled != (change != "disabled") {
			t.Fatalf("%s: %+v", change, got)
		}
	}
}

func TestChannelDiagnosticsRoutingEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, verdict, actual string
		status                int
	}{
		{"matched", "matched", "other", 200},
		{"ignored", "ignored", "base", 200},
		{"unsupported", "unsupported", "", 403},
		{"upstream-refused", "unknown", "", 502},
		{"no-alternative", "unknown", "base", 200},
	} {
		for _, pipeline := range probePipelines {
			t.Run(tc.name+"/"+pipeline, func(t *testing.T) {
				cfg := &config.Config{ClinePass: config.ClinePassConfig{BaseURL: "https://synthetic.invalid/chat/completions", APIKeys: []string{"synthetic-credential"}, ChannelProbeEnabled: true}}
				p := NewClinePassProvider(config.NewAtomicConfig(cfg, ""), nil)
				calls := 0
				p.httpClient.Transport = responseContractTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					_, _ = io.Copy(io.Discard, r.Body)
					status, served := tc.status, tc.actual
					candidates := []string{}
					if calls == 1 {
						status, served = 200, "base"
						candidates = []string{"base", "other"}
						if tc.name == "no-alternative" {
							candidates = []string{"base"}
						}
					}
					body := strings.Replace(routingFrame(served, candidates...), `"finalProvider":"`+served+`"`, `"finalProvider":"`+served+`","canonicalSlug":"`+served+`/synthetic"`, 1)
					if status == 403 {
						body = `{"error":{"message":"client-supplied \"providerOptions\" model routing is not supported"}}`
					} else if status == 502 {
						body = "synthetic-private-error-detail"
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
				})
				p.probeOneModel(t.Context(), cfg, cfg.ClinePass.APIKeys, "synthetic-model", pipeline)
				row := p.ChannelDiagnostics().Results[0]
				wantTarget, wantCanonical := "other", ""
				if tc.name == "no-alternative" {
					wantTarget = ""
				}
				if tc.actual != "" {
					wantCanonical = tc.actual + "/synthetic"
				}
				if row.Verdict != tc.verdict || row.Baseline != "base" || row.Target != wantTarget || row.Actual != tc.actual || row.CanonicalSlug != wantCanonical {
					t.Fatalf("routing evidence = %+v", row)
				}
				encoded, err := json.Marshal(row)
				if err != nil || strings.Contains(string(encoded), cfg.ClinePass.APIKeys[0]) || strings.Contains(string(encoded), "synthetic-private-error-detail") {
					t.Fatal("routing evidence exposed private fields")
				}
			})
		}
	}
}

func TestChannelDiagnosticsLateRoundCannotPublishForChangedScope(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var first sync.Once
	srv := probeUpstream(t, []string{"synthetic"}, func(w http.ResponseWriter, r *http.Request) {
		first.Do(func() { close(entered) })
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, routingFrame("minimax"))
	})
	cfg := probeConfig(t, srv)
	a := config.NewAtomicConfig(cfg, "")
	p := NewClinePassProvider(a, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	go func() { defer close(done); p.runChannelPinProbe(ctx) }()
	select {
	case <-entered:
	case <-ctx.Done():
		close(release)
		t.Fatal("probe did not start")
	}
	next := *cfg
	next.ClinePass.APIKey = "synthetic-changed"
	a.ApplyLoaded(&next)
	close(release)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("probe did not finish")
	}
	if len(p.ChannelDiagnostics().Results) != 0 {
		t.Fatal("late old-scope evidence published for changed key")
	}
}

func TestChannelDiagnosticsStaleRequestCannotClearCurrentScope(t *testing.T) {
	for _, change := range []string{"endpoint", "key"} {
		t.Run(change, func(t *testing.T) {
			srv := probeUpstream(t, []string{"synthetic"}, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, routingFrame("minimax"))
			})
			cfg := probeConfig(t, srv)
			cfg.Models = map[string]config.ModelConfig{"default": {Provider: "cline-pass", ModelID: "synthetic"}}
			a := config.NewAtomicConfig(cfg, "")
			p := NewClinePassProvider(a, nil)
			captured := a.Get()
			next := *cfg
			if change == "endpoint" {
				next.ClinePass.BaseURL += "?scope=new"
			} else {
				next.ClinePass.APIKey = "synthetic-new-key"
			}
			a.ApplyLoaded(&next)
			p.rememberProbeModel(a.Get(), "synthetic")
			p.runChannelPinProbe(t.Context())
			before := p.ChannelDiagnostics()
			if len(before.Results) != len(probePipelines) {
				t.Fatalf("new scope did not publish evidence: %+v", before)
			}
			// A request captured its config before the new scope was published.
			p.rememberProbeModel(captured, "old-model")
			if got := p.ChannelDiagnostics(); !reflect.DeepEqual(got, before) {
				t.Fatalf("stale request cleared current evidence: %+v", got)
			}
			if got := p.clinePassProbeModels(t.Context(), a.Get()); !reflect.DeepEqual(got, []string{"synthetic"}) {
				t.Fatalf("stale request changed current models: %v", got)
			}
		})
	}
}
