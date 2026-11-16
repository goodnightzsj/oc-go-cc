package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestChannelProbeAliasIsNotAnAlternative(t *testing.T) {
	var calls atomic.Int32
	srv := probeUpstream(t, []string{"synthetic"}, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		target := plannerPin(request)
		if target == "" {
			target = directPin(request)
		}
		if target == "" {
			_, _ = io.WriteString(w, routingFrame("zai", "z-ai"))
		} else {
			_, _ = io.WriteString(w, routingFrame("z-ai"))
		}
	})
	p := NewClinePassProvider(config.NewAtomicConfig(probeConfig(t, srv), ""), nil)
	p.runChannelPinProbe(t.Context())
	results := p.ChannelDiagnostics().Results
	if len(results) != 2 {
		t.Fatalf("missing pipeline evidence: %+v", results)
	}
	for _, result := range results {
		if result.Verdict != "unknown" || result.Reason != "no_alternative" {
			t.Errorf("same-channel alias treated as alternative: %+v", result)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("no alternative must avoid paid variants: calls=%d", got)
	}
}

func TestChannelProbeMatchesEquivalentAlias(t *testing.T) {
	srv := probeUpstream(t, []string{"synthetic"}, func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		target := plannerPin(request)
		if target == "" {
			target = directPin(request)
		}
		if target == "" {
			_, _ = io.WriteString(w, routingFrame("minimax", "minimax", "z-ai"))
		} else {
			_, _ = io.WriteString(w, routingFrame("zai"))
		}
	})
	got := probeVerdict(t, probeConfig(t, srv))
	if strings.Count(got, "verdict=matched") != 2 || strings.Contains(got, "verdict=ignored") {
		t.Fatalf("equivalent alias incorrectly classified: %s", got)
	}
}

func TestChannelCandidateAliasDeduplication(t *testing.T) {
	_, _, candidates, err := routingSummary(routingFrame("zai", "z-ai", "zai", "nebius"))
	if err != nil || strings.Join(candidates, ",") != "z-ai,nebius" || firstCandidateOtherThan(candidates, "zai") != "nebius" {
		t.Fatalf("wrong candidate identity: candidates=%v err=%v", candidates, err)
	}
}
