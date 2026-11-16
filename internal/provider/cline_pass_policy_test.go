package provider

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/router"
)

func TestClinePassRequiredChannelFailsBeforeHTTP(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmtStream(stream), func(t *testing.T) {
			calls := 0
			p, _ := clinePassTestProvider(t, func(http.ResponseWriter, *http.Request) { calls++ })
			cfg := *p.atomic.Get()
			cfg.ClinePass.ChannelPinEnabled = true
			cfg.ClinePass.ChannelPin = "private"
			cfg.ClinePass.ChannelPinCapabilities = map[string][]string{"synthetic": {"vision"}}
			p.atomic.ApplyLoaded(&cfg)
			model := config.ModelConfig{Provider: p.Name(), ModelID: "synthetic"}
			var err error
			if stream {
				_, err = p.Stream(t.Context(), clinePassRequest(), model)
			} else {
				_, err = p.Execute(t.Context(), clinePassRequest(), model)
			}
			var apiErr *client.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(apiErr.Body, "channel_pin_unsupported") {
				t.Fatalf("missing explicit pin policy error: %v", err)
			}
			if calls != 0 || len(p.keys.states) != 0 || !router.IsNonRetryableRequestError(err) {
				t.Fatalf("unsupported policy spent quota, touched health or allowed fallback: calls=%d states=%d err=%v", calls, len(p.keys.states), err)
			}
		})
	}
}

func TestClinePassMatchedProbeNeverEnablesRequiredRouting(t *testing.T) {
	var calls atomic.Int32
	srv := probeUpstream(t, []string{"synthetic"}, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		target := plannerPin(request)
		if target == "" {
			target = directPin(request)
		}
		if target == "" {
			_, _ = io.WriteString(w, routingFrame("base", "base", "private"))
		} else {
			_, _ = io.WriteString(w, routingFrame(target))
		}
	})
	cfg := probeConfig(t, srv)
	cfg.ClinePass.APIKey = "synthetic"
	cfg.Models = map[string]config.ModelConfig{"default": {Provider: "cline-pass", ModelID: "synthetic"}}
	p := NewClinePassProvider(config.NewAtomicConfig(cfg, ""), nil)
	p.runChannelPinProbe(t.Context())
	snapshot := p.ChannelDiagnostics()
	if len(snapshot.Results) != 2 || snapshot.EnforcementVerified || calls.Load() != 4 {
		t.Fatalf("probe fixture did not complete both paths: %+v calls=%d", snapshot, calls.Load())
	}
	for _, result := range snapshot.Results {
		if result.Verdict != "matched" || result.Target != "private" || result.Actual != "private" {
			t.Fatalf("fixture must demonstrate matched evidence: %+v", result)
		}
	}
	blocked := p.keys.blockedUntil("synthetic")
	next := *cfg
	next.ClinePass.ChannelPinEnabled, next.ClinePass.ChannelPin = true, "private"
	p.atomic.ApplyLoaded(&next)
	model := cfg.Models["default"]
	_, executeErr := p.Execute(t.Context(), clinePassRequest(), model)
	_, streamErr := p.Stream(t.Context(), clinePassRequest(), model)
	for _, err := range []error{executeErr, streamErr} {
		var apiErr *client.APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != 422 || !strings.Contains(apiErr.Body, "channel_pin_unsupported") {
			t.Fatalf("matched probe authorized required routing: %v", err)
		}
	}
	if calls.Load() != 4 || p.keys.blockedUntil("synthetic") != blocked {
		t.Fatal("required routing spent quota or changed key health after matched probe")
	}
}
