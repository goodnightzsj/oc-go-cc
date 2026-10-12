package provider

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
)

// lineWriter collects the probe's log lines so a test can assert on the
// verdict rather than on internal state. The verdict IS the product here.
type lineWriter struct {
	append func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.append(string(p))
	return len(p), nil
}

// rosterJSON is the shape the platform publishes at its model-list endpoint.
func rosterJSON(ids ...string) string {
	entries := make([]string, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, `{"id":"`+id+`","name":"`+id+`"}`)
	}
	return `{"clinePass":[` + strings.Join(entries, ",") + `]}`
}

// probeUpstream serves both endpoints the probe touches: the model roster and
// the completion itself. They are one server because the probe derives the
// roster URL from the configured API endpoint.
func probeUpstream(t *testing.T, roster []string, completion http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/recommended-models") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, rosterJSON(roster...))
			return
		}
		completion(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// probeConfig points the platform at a test server, with pinning on.
func probeConfig(t *testing.T, srv *httptest.Server) *config.Config {
	t.Helper()
	return &config.Config{
		ClinePass: config.ClinePassConfig{
			APIKey:            "cline-key",
			BaseURL:           srv.URL + "/api/v1/chat/completions",
			ChannelPinEnabled: true,
			ChannelPin:        "deepseek",
		},
	}
}

// probeVerdict runs the probe and returns everything it logged.
func probeVerdict(t *testing.T, cfg *config.Config) string {
	t.Helper()
	p := NewClinePassProvider(config.NewAtomicConfig(cfg, ""), nil)

	var mu sync.Mutex
	var captured []string
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&lineWriter{append: func(s string) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, s)
	}}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	p.runChannelPinProbe(ctx)

	mu.Lock()
	defer mu.Unlock()
	return strings.Join(captured, "\n")
}

// The three verdicts are the whole contract. Each one must be reachable, and
// each must be distinguishable from the others: an "enforced" read as
// "ignored" would hide a working pin, and the reverse would invent one.
func TestChannelPinProbeReportsWhetherTheGatewayStillReadsTheField(t *testing.T) {
	model := "cline-pass/deepseek-v4.1-flash"

	t.Run("enforced: the impossible channel is refused", func(t *testing.T) {
		srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"Available providers are: alibaba, deepseek, fireworks"}}`)
		})
		got := probeVerdict(t, probeConfig(t, srv))
		if !strings.Contains(got, "verdict=enforced") {
			t.Errorf("a rejection means the field is read; got:\n%s", got)
		}
		if !strings.Contains(got, "alibaba, deepseek, fireworks") {
			t.Errorf("the candidate list from the rejection must be kept; got:\n%s", got)
		}
	})

	t.Run("ignored: the impossible channel is accepted", func(t *testing.T) {
		srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":\"openai-compatible-private\"}}}}}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		})
		got := probeVerdict(t, probeConfig(t, srv))
		if !strings.Contains(got, "verdict=ignored") {
			t.Errorf("a 200 for an impossible channel means the field is dropped; got:\n%s", got)
		}
		if !strings.Contains(got, "openai-compatible-private") {
			t.Errorf("the probe must report where the gateway actually routed; got:\n%s", got)
		}
	})

	t.Run("unknown: transport failure is not a verdict", func(t *testing.T) {
		dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		base := dead.URL + "/api/v1/chat/completions"
		dead.Close()
		cfg := &config.Config{ClinePass: config.ClinePassConfig{
			APIKey: "cline-key", BaseURL: base, ChannelPinEnabled: true, ChannelPin: "deepseek",
		}}
		got := probeVerdict(t, cfg)
		// No roster means no models, which is a skip rather than a verdict. The
		// requirement is only that nothing is claimed about pinning.
		if strings.Contains(got, "verdict=enforced") || strings.Contains(got, "verdict=ignored") {
			t.Errorf("a transport failure must not become a routing verdict; got:\n%s", got)
		}
	})

	t.Run("unknown: empty response content is a token-budget problem, not a verdict", func(t *testing.T) {
		// Reported by the upstream as HTTP 500. Reading it as "enforced" would
		// be wrong: nothing was refused, the model just had no room to answer.
		srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"empty response content"}`)
		})
		got := probeVerdict(t, probeConfig(t, srv))
		if !strings.Contains(got, "verdict=unknown") {
			t.Errorf("empty response content is not a routing verdict; got:\n%s", got)
		}
	})
}

// Pinning off means no probe at all: the probe spends a real request against a
// metered quota, and there is nothing to explain when nothing is pinned.
func TestChannelPinProbeIsSilentWhenPinningIsOff(t *testing.T) {
	called := false
	srv := probeUpstream(t, []string{"cline-pass/deepseek-v4.1-flash"}, func(http.ResponseWriter, *http.Request) {
		called = true
	})
	cfg := probeConfig(t, srv)
	cfg.ClinePass.ChannelPinEnabled = false

	got := probeVerdict(t, cfg)
	if called {
		t.Error("pinning is off, so no completion request may be made")
	}
	if strings.Contains(got, "verdict=") {
		t.Errorf("pinning is off, so no verdict may be logged; got:\n%s", got)
	}
}

// The model list must come from the platform's roster, not from cfg.Models or
// the overrides.
//
// This is the bug this test exists for: on the deployment it was written for,
// cfg.Models held only CommandCode entries and the ClinePass ids arrived through
// model_overrides, so a config scan found nothing and the probe silently did no
// work - it deployed, ran, and logged nothing at all.
func TestChannelPinProbeTakesModelsFromThePlatformRoster(t *testing.T) {
	probed := map[string]bool{}
	srv := probeUpstream(t, []string{"cline-pass/deepseek-v4.1-flash"}, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		probed[body.Model] = true
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":\"x\"}}}}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})

	cfg := probeConfig(t, srv)
	// A config whose Models belong to another platform, exactly like the
	// deployment: the roster must still be what gets probed.
	cfg.Models = map[string]config.ModelConfig{
		"default": {Provider: site.CommandCode, ModelID: "deepseek/deepseek-v4-pro"},
	}
	cfg.ModelOverrides = map[string]config.ModelConfig{
		"claude-sonnet-4-6": {Provider: site.CommandCode, ModelID: "deepseek/deepseek-v4-pro"},
	}

	got := probeVerdict(t, cfg)
	if !probed["cline-pass/deepseek-v4.1-flash"] {
		t.Errorf("the roster model must be probed; probed=%v\nlog:\n%s", probed, got)
	}
	if probed["deepseek/deepseek-v4-pro"] {
		t.Error("a model from another platform must never be probed")
	}
	if strings.Count(got, "verdict=") != 1 {
		t.Errorf("expected one verdict per roster model; got:\n%s", got)
	}
}

// The probe must actually put the impossible channel on the wire, otherwise it
// is measuring nothing.
func TestChannelPinProbeSendsTheImpossibleChannel(t *testing.T) {
	var seen map[string]any
	srv := probeUpstream(t, []string{"cline-pass/deepseek-v4.1-flash"}, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&seen)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"No allowed providers are available"}}`)
	})
	_ = probeVerdict(t, probeConfig(t, srv))

	if seen == nil {
		t.Fatal("no request body captured")
	}
	opts, ok := seen["providerOptions"].(map[string]any)
	if !ok {
		t.Fatalf("providerOptions missing from the probe request: %v", seen)
	}
	gateway, _ := opts["gateway"].(map[string]any)
	only, _ := gateway["only"].([]any)
	if len(only) != 1 || only[0] != probeSlug {
		t.Errorf("the probe must send only=[%q]; got %v", probeSlug, only)
	}
	// Small max_tokens makes a reasoning model answer "empty response content",
	// which is indistinguishable from a routing refusal.
	if n, _ := seen["max_tokens"].(float64); n < 32 {
		t.Errorf("max_tokens must clear the reasoning budget; got %v", seen["max_tokens"])
	}
}
