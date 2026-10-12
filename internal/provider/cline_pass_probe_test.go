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

// probeConfig builds a config whose cline-pass models are the given ids, with
// pinning on and the base URL pointed at a test upstream.
func probeConfig(t *testing.T, baseURL string, pinEnabled bool, modelIDs ...string) *config.Config {
	t.Helper()
	models := map[string]config.ModelConfig{}
	for _, id := range modelIDs {
		models[id] = config.ModelConfig{Provider: site.ClinePass, ModelID: id}
	}
	// A model from another platform must never be probed.
	models["opencode-go/glm-5.2"] = config.ModelConfig{Provider: site.OpenCodeGo, ModelID: "opencode-go/glm-5.2"}
	return &config.Config{
		Models: models,
		ClinePass: config.ClinePassConfig{
			APIKey:            "cline-key",
			BaseURL:           baseURL,
			ChannelPinEnabled: pinEnabled,
			ChannelPin:        "deepseek",
		},
	}
}

// probeVerdict runs the probe against a fake upstream and returns the logged
// line.
func probeVerdict(t *testing.T, cfg *config.Config, handler http.HandlerFunc) string {
	t.Helper()
	upstream := httptest.NewServer(handler)
	defer upstream.Close()
	cfg.ClinePass.BaseURL = upstream.URL

	p := NewClinePassProvider(config.NewAtomicConfig(cfg, ""), nil)

	var captured []string
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&lineWriter{append: func(s string) {
		captured = append(captured, s)
	}}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p.runChannelPinProbe(ctx)

	return strings.Join(captured, "\n")
}

// The three verdicts are the whole contract. Each one must be reachable, and
// each must be distinguishable from the others: an "enforced" read as
// "ignored" would hide a working pin, and the reverse would invent one.
func TestChannelPinProbeReportsWhetherTheGatewayStillReadsTheField(t *testing.T) {
	t.Run("enforced: the impossible channel is refused", func(t *testing.T) {
		cfg := probeConfig(t, "", true, "cline-pass/deepseek-v4.1-flash")
		got := probeVerdict(t, cfg, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"Available providers are: alibaba, deepseek, fireworks"}}`)
		})
		if !strings.Contains(got, "verdict=enforced") {
			t.Errorf("a rejection means the field is read; got:\n%s", got)
		}
		if !strings.Contains(got, "alibaba, deepseek, fireworks") {
			t.Errorf("the candidate list from the rejection must be kept; got:\n%s", got)
		}
	})

	t.Run("ignored: the impossible channel is accepted", func(t *testing.T) {
		cfg := probeConfig(t, "", true, "cline-pass/deepseek-v4.1-flash")
		got := probeVerdict(t, cfg, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":\"openai-compatible-private\"}}}}}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		})
		if !strings.Contains(got, "verdict=ignored") {
			t.Errorf("a 200 for an impossible channel means the field is dropped; got:\n%s", got)
		}
		if !strings.Contains(got, "openai-compatible-private") {
			t.Errorf("the probe must report where the gateway actually routed; got:\n%s", got)
		}
	})

	t.Run("unknown: transport failure is not a verdict", func(t *testing.T) {
		cfg := probeConfig(t, "", true, "cline-pass/deepseek-v4.1-flash")
		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		upstream.Close() // refuse connections
		cfg.ClinePass.BaseURL = upstream.URL
		p := NewClinePassProvider(config.NewAtomicConfig(cfg, ""), nil)

		var captured []string
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&lineWriter{append: func(s string) {
			captured = append(captured, s)
		}}, &slog.HandlerOptions{Level: slog.LevelDebug})))
		defer slog.SetDefault(previous)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.runChannelPinProbe(ctx)

		got := strings.Join(captured, "\n")
		if !strings.Contains(got, "verdict=unknown") {
			t.Errorf("a transport failure must not be reported as a routing verdict; got:\n%s", got)
		}
	})

	t.Run("unknown: empty response content is a token-budget problem, not a verdict", func(t *testing.T) {
		// Reported by the upstream as HTTP 500. Reading it as "enforced" would
		// be wrong: nothing was refused, the model just had no room to answer.
		cfg := probeConfig(t, "", true, "cline-pass/deepseek-v4.1-flash")
		got := probeVerdict(t, cfg, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"empty response content"}`)
		})
		if !strings.Contains(got, "verdict=unknown") {
			t.Errorf("empty response content is not a routing verdict; got:\n%s", got)
		}
	})
}

// Pinning off means no probe at all: the probe spends a real request against a
// metered quota, and there is nothing to explain when nothing is pinned.
func TestChannelPinProbeIsSilentWhenPinningIsOff(t *testing.T) {
	called := false
	cfg := probeConfig(t, "", false, "cline-pass/deepseek-v4.1-flash")
	got := probeVerdict(t, cfg, func(http.ResponseWriter, *http.Request) {
		called = true
	})
	if called {
		t.Error("pinning is off, so no upstream request may be made")
	}
	if strings.Contains(got, "verdict=") {
		t.Errorf("pinning is off, so no verdict may be logged; got:\n%s", got)
	}
}

// The probe must read the model list from the config, not a hard-coded name,
// and must not touch models belonging to other platforms.
func TestChannelPinProbeCoversConfiguredClinePassModelsOnly(t *testing.T) {
	cfg := probeConfig(t, "", true, "cline-pass/deepseek-v4.1-flash", "cline-pass/glm-5.2")
	// The probe fans out one goroutine per model, so the handlers run
	// concurrently and this map needs a lock.
	var mu sync.Mutex
	probed := map[string]bool{}
	got := probeVerdict(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		probed[body.Model] = true
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":\"x\"}}}}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	mu.Lock()
	defer mu.Unlock()
	if !probed["cline-pass/deepseek-v4.1-flash"] || !probed["cline-pass/glm-5.2"] {
		t.Errorf("both configured cline-pass models must be probed; probed=%v", probed)
	}
	if probed["opencode-go/glm-5.2"] {
		t.Error("a model from another platform must never be probed")
	}
	if strings.Count(got, "verdict=") != 2 {
		t.Errorf("expected one verdict per cline-pass model; got:\n%s", got)
	}
}

// The probe must actually put the impossible channel on the wire, otherwise it
// is measuring nothing.
func TestChannelPinProbeSendsTheImpossibleChannel(t *testing.T) {
	cfg := probeConfig(t, "", true, "cline-pass/deepseek-v4.1-flash")
	var seen map[string]any
	_ = probeVerdict(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&seen)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"No allowed providers are available"}}`)
	})
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
