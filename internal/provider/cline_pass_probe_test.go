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


// routingFrame builds the SSE tail the gateway sends: the routing block, in the
// last frame before [DONE].
func routingFrame(served string, candidates ...string) string {
	quoted := make([]string, 0, len(candidates))
	for _, c := range candidates {
		quoted = append(quoted, `"`+c+`"`)
	}
	block := `{"finalProvider":"` + served + `","fallbacksAvailable":[` + strings.Join(quoted, ",") + `]}`
	return "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"provider_metadata\":{\"gateway\":{\"routing\":" + block + "}}}}]}\n\n" +
		"data: [DONE]\n\n"
}

// pinAwareUpstream answers the baseline (unpinned) request with a candidate
// list and the pinned request according to how the test wants the gateway to
// behave: honoured sends it to the pinned channel, ignored keeps it on the
// baseline channel.
//
// This is the shape the probe actually has to work against, and it is why the
// impossible channel cannot answer the question: both behaviours look the same
// to a sentinel probe, because an unservable pin is silently accepted either
// way.
func pinAwareUpstream(t *testing.T, baseline string, candidates []string, honoured bool, pinned func(body map[string]any) string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		target := pinned(body)
		if target == "" {
			_, _ = io.WriteString(w, routingFrame(baseline, candidates...))
			return
		}
		if honoured {
			_, _ = io.WriteString(w, routingFrame(target))
			return
		}
		_, _ = io.WriteString(w, routingFrame(baseline))
	}
}

// plannerPin reads the pinned channel out of the planner pipeline's field.
func plannerPin(body map[string]any) string {
	opts, _ := body["providerOptions"].(map[string]any)
	gateway, _ := opts["gateway"].(map[string]any)
	only, _ := gateway["only"].([]any)
	if len(only) == 1 {
		s, _ := only[0].(string)
		return s
	}
	return ""
}

// directPin reads the pinned channel out of the direct pipeline's field.
func directPin(body map[string]any) string {
	prov, _ := body["provider"].(map[string]any)
	only, _ := prov["only"].([]any)
	if len(only) == 1 {
		s, _ := only[0].(string)
		return s
	}
	return ""
}

// The verdict is a comparison, not a sentinel: the probe must pin to a channel
// the baseline did NOT use, and read where the pinned request was served.
func TestChannelPinProbeComparesPinnedAgainstUnpinned(t *testing.T) {
	model := "cline-pass/minimax-m3"

	t.Run("enforced: the pinned request moves to the pinned channel", func(t *testing.T) {
		srv := probeUpstream(t, []string{model}, pinAwareUpstream(t, "minimax",
			[]string{"minimax", "nebius", "gmicloud"}, true, plannerPin))
		got := probeVerdict(t, probeConfig(t, srv))
		if !strings.Contains(got, "verdict=enforced") {
			t.Errorf("a followed pin is enforcement; got:\n%s", got)
		}
		if !strings.Contains(got, "pin to nebius honoured") {
			t.Errorf("the probe must name the channel it pinned and where it landed; got:\n%s", got)
		}
	})

	t.Run("ignored: the pinned request stays on the baseline channel", func(t *testing.T) {
		srv := probeUpstream(t, []string{model}, pinAwareUpstream(t, "minimax",
			[]string{"minimax", "nebius", "gmicloud"}, false, plannerPin))
		got := probeVerdict(t, probeConfig(t, srv))
		if !strings.Contains(got, "verdict=ignored") {
			t.Errorf("a pin that does not move the request is ignored; got:\n%s", got)
		}
		if !strings.Contains(got, "still served by minimax") {
			t.Errorf("the probe must report where it actually landed; got:\n%s", got)
		}
	})

	// Pinning to the channel the request would have used anyway proves nothing:
	// the gateway looks obedient while having made no decision. The target must
	// be a candidate the baseline did not use.
	t.Run("the target is never the baseline channel", func(t *testing.T) {
		var pinnedTo []string
		srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "text/event-stream")
			if target := plannerPin(body); target != "" {
				pinnedTo = append(pinnedTo, target)
				_, _ = io.WriteString(w, routingFrame(target))
				return
			}
			_, _ = io.WriteString(w, routingFrame("minimax", "minimax", "nebius"))
		})
		_ = probeVerdict(t, probeConfig(t, srv))
		if len(pinnedTo) != 1 || pinnedTo[0] != "nebius" {
			t.Errorf("must pin to the first candidate other than the baseline; pinned to %v", pinnedTo)
		}
	})

	// A model with one channel has nothing to pin to. Reporting that as
	// "ignored" would blame the gateway for a model that has nowhere else to go.
	t.Run("locked: a single-channel model has nothing to pin to", func(t *testing.T) {
		srv := probeUpstream(t, []string{model}, pinAwareUpstream(t, "minimax", []string{"minimax"}, true, plannerPin))
		got := probeVerdict(t, probeConfig(t, srv))
		if !strings.Contains(got, "verdict=locked") {
			t.Errorf("a one-channel model is locked, not ignored; got:\n%s", got)
		}
	})

	// The real account's shape: a private route reports no candidates at all.
	t.Run("locked: a private route with no candidates", func(t *testing.T) {
		srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, routingFrame("openai-compatible-private"))
		})
		got := probeVerdict(t, probeConfig(t, srv))
		if !strings.Contains(got, "verdict=locked") {
			t.Errorf("a private route cannot be pinned; got:\n%s", got)
		}
	})
}

// An exhausted quota answers every probe with 429. Reading that as "enforced"
// would report proof that pinning works, from an account that routed nothing.
// Two keys on the same host produced opposite verdicts for the same question,
// and the wrong one was the reassuring answer.
func TestChannelPinProbeDoesNotReadAccountRefusalsAsVerdicts(t *testing.T) {
	model := "cline-pass/minimax-m3"

	t.Run("an exhausted quota is unknown", func(t *testing.T) {
		srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"code":"INFERENCE_CAP_ERROR","message":"You have reached your monthly Clinepass limit."}}`)
		})
		got := probeVerdict(t, probeConfig(t, srv))
		if strings.Contains(got, "verdict=enforced") || strings.Contains(got, "verdict=ignored") {
			t.Errorf("a quota refusal says nothing about pinning; got:\n%s", got)
		}
		if !strings.Contains(got, "INFERENCE_CAP_ERROR") {
			t.Errorf("the gateway's own code must be reported; got:\n%s", got)
		}
	})

	t.Run("a rejected credential is unknown", func(t *testing.T) {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusPaymentRequired} {
			srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":{"message":"nope"}}`)
			})
			got := probeVerdict(t, probeConfig(t, srv))
			if !strings.Contains(got, "verdict=unknown") {
				t.Errorf("HTTP %d is about the credential, not routing; got:\n%s", status, got)
			}
		}
	})

	t.Run("empty response content is unknown", func(t *testing.T) {
		srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"empty response content"}`)
		})
		got := probeVerdict(t, probeConfig(t, srv))
		if strings.Contains(got, "verdict=enforced") {
			t.Errorf("a token-budget failure is not a routing verdict; got:\n%s", got)
		}
	})
}

// Each pipeline must carry only its own field, and the probe must ask both.
func TestChannelPinProbeSeparatesTheTwoPipelines(t *testing.T) {
	model := "cline-pass/minimax-m3"
	var mu sync.Mutex
	var bodies []map[string]any
	srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		// Every request gets candidates, so both pipelines run the full
		// baseline+pinned pair.
		_, _ = io.WriteString(w, routingFrame("minimax", "minimax", "nebius"))
	})
	_ = probeVerdict(t, probeConfig(t, srv))

	mu.Lock()
	defer mu.Unlock()
	// Two pipelines x (baseline + pinned).
	if len(bodies) != 4 {
		t.Fatalf("expected 4 requests (2 pipelines x baseline+pinned); got %d", len(bodies))
	}

	var sawPlannerPin, sawDirectPin, sawBaseline bool
	for _, body := range bodies {
		_, hasOpts := body["providerOptions"]
		_, hasProv := body["provider"]
		if hasOpts && hasProv {
			t.Errorf("a probe must carry exactly one pipeline's field; body = %v", body)
		}
		switch {
		case !hasOpts && !hasProv:
			sawBaseline = true
		case hasOpts:
			sawPlannerPin = true
		case hasProv:
			sawDirectPin = true
		}
	}
	if !sawBaseline {
		t.Error("the unpinned baseline must be sent: without it there is nothing to compare against")
	}
	if !sawPlannerPin || !sawDirectPin {
		t.Errorf("both pipelines must be probed; planner=%v direct=%v", sawPlannerPin, sawDirectPin)
	}
}

// The direct pipeline names the serving channel resolvedProvider rather than
// finalProvider, and publishes its candidate list as JSON metadata. Reading
// only the planner's spellings makes its answer look like no metadata at all.
func TestChannelPinProbeReadsTheDirectPipelineSpellings(t *testing.T) {
	model := "cline-pass/minimax-m3"
	var mu sync.Mutex
	pinnedTo := ""
	srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		if _, ok := body["provider"]; !ok {
			_, _ = io.WriteString(w, routingFrame("minimax", "minimax", "nebius"))
			return
		}
		mu.Lock()
		pinnedTo = directPin(body)
		mu.Unlock()
		// resolvedProvider, not finalProvider.
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"provider_metadata\":{\"gateway\":{\"routing\":{\"resolvedProvider\":\"nebius\"}}}}}]}\n\n"+
			"data: [DONE]\n\n")
	})
	got := probeVerdict(t, probeConfig(t, srv))

	mu.Lock()
	defer mu.Unlock()
	if pinnedTo != "nebius" {
		t.Errorf("the direct pipeline must pin provider.only; pinned to %q", pinnedTo)
	}
	if !strings.Contains(got, "pin to nebius honoured") {
		t.Errorf("resolvedProvider must be read as the serving channel; got:\n%s", got)
	}
}

// The probe must actually put a pin on the wire, and the token budget must
// clear the reasoning pass.
func TestChannelPinProbeSendsAServablePin(t *testing.T) {
	model := "cline-pass/minimax-m3"
	var seen map[string]any
	srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if target := plannerPin(body); target != "" {
			seen = body
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, routingFrame("minimax", "minimax", "nebius"))
	})
	_ = probeVerdict(t, probeConfig(t, srv))

	if seen == nil {
		t.Fatal("no pinned request was sent")
	}
	// The pin must be a real channel, not an impossible sentinel: an unservable
	// pin is accepted and silently falls back, so it cannot test anything.
	if got := plannerPin(seen); got != "nebius" {
		t.Errorf("the probe must pin to a servable candidate; got %q", got)
	}
	// Small max_tokens makes a reasoning model answer "empty response content",
	// which is indistinguishable from a routing refusal.
	if n, _ := seen["max_tokens"].(float64); n < 32 {
		t.Errorf("max_tokens must clear the reasoning budget; got %v", seen["max_tokens"])
	}
}

// A pin to a channel the model cannot serve is a hard failure, and the gateway
// reports it inside the stream with HTTP 200 rather than as a status code:
//
//	data: {"error":{"code":"stream_initialization_failed", ...}}
//
// Read as "no routing metadata" that would be unknown, but it is in fact the
// strongest evidence of enforcement - the gateway read the pin and tried to
// honour it, then could not find the channel.
func TestChannelPinProbeReadsInStreamErrors(t *testing.T) {
	model := "cline-pass/minimax-m3"
	srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		if target := plannerPin(body); target != "" {
			// HTTP 200, error inside the stream.
			_, _ = io.WriteString(w, "data: {\"error\":{\"code\":\"stream_initialization_failed\",\"message\":\"failed to invoke model 'minimax/minimax-m3'\"}}\n\n")
			return
		}
		_, _ = io.WriteString(w, routingFrame("minimax", "minimax", "nebius"))
	})
	got := probeVerdict(t, probeConfig(t, srv))

	if !strings.Contains(got, "verdict=enforced") {
		t.Errorf("an in-stream rejection of the pin proves enforcement; got:\n%s", got)
	}
	if !strings.Contains(got, "stream_initialization_failed") {
		t.Errorf("the gateway's error code must be reported; got:\n%s", got)
	}
	// The upstream vendor's wording must not be copied into the log.
	if strings.Contains(got, "failed to invoke model") {
		t.Errorf("the upstream message must not be logged, only its code; got:\n%s", got)
	}
}

// An error frame on the UNPINNED baseline is not about pinning: there is no pin
// to enforce. It must stay unknown rather than being read as enforcement.
func TestChannelPinProbeDoesNotReadBaselineErrorsAsEnforcement(t *testing.T) {
	model := "cline-pass/minimax-m3"
	srv := probeUpstream(t, []string{model}, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"error\":{\"code\":\"stream_initialization_failed\"}}\n\n")
	})
	got := probeVerdict(t, probeConfig(t, srv))
	if strings.Contains(got, "verdict=enforced") {
		t.Errorf("a baseline failure says nothing about pinning; got:\n%s", got)
	}
}
