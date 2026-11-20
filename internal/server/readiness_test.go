package server

import (
	"testing"

	"github.com/routatic/proxy/internal/config"
)

type stubAvailability map[string]bool

func (s stubAvailability) CredentialsExhausted(provider string) bool { return s[provider] }

// An empty active_site means "any platform may serve". Treating it as "the
// default platform must have a key" made a healthy instance answer 503, and the
// deploy gate rolls back on 503.
func TestReadinessProbeRespectsUnrestrictedRouting(t *testing.T) {
	for _, tc := range []struct {
		name       string
		activeSite string
		exhausted  map[string]bool
		want       bool
	}{
		{
			name:       "unrestricted with a platform that can serve",
			activeSite: "",
			exhausted:  map[string]bool{"opencode-go": true, "commandcode": true, "cline-pass": false},
			want:       false,
		},
		{
			name:       "unrestricted with nothing anywhere",
			activeSite: "",
			exhausted:  map[string]bool{"opencode-go": true, "commandcode": true, "cline-pass": true, "workbuddy": true, "opencode-zen": true, "aws-bedrock": true, "openrouter": true},
			want:       true,
		},
		{
			name:       "pinned site without a credential",
			activeSite: "cline-pass",
			exhausted:  map[string]bool{"cline-pass": true, "opencode-go": false},
			want:       true,
		},
		{
			name:       "pinned site with a credential",
			activeSite: "cline-pass",
			exhausted:  map[string]bool{"cline-pass": false, "opencode-go": true},
			want:       false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			atomic := config.NewAtomicConfig(&config.Config{ActiveSite: tc.activeSite}, "")
			_, degraded := readinessProbe(atomic, stubAvailability(tc.exhausted))()
			if degraded != tc.want {
				t.Fatalf("degraded = %v, want %v", degraded, tc.want)
			}
		})
	}
}

// The direct passthrough owns its own availability gate, which /health cannot
// see. Reporting healthy is the only honest answer there.
func TestReadinessProbeStaysQuietForTheDirectPath(t *testing.T) {
	atomic := config.NewAtomicConfig(&config.Config{
		ActiveSite:     "cline-pass",
		AnthropicFirst: config.AnthropicFirstConfig{Enabled: true},
	}, "")
	if _, degraded := readinessProbe(atomic, stubAvailability{"cline-pass": true})(); degraded {
		t.Fatal("readiness judged a route it cannot observe")
	}
}
