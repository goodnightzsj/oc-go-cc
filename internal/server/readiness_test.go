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
			_, degraded := readinessProbe(atomic, stubAvailability(tc.exhausted), nil)()
			if degraded != tc.want {
				t.Fatalf("degraded = %v, want %v", degraded, tc.want)
			}
		})
	}
}

// With the direct passthrough enabled, the platform credentials are only a
// fallback. A healthy gate means the instance serves regardless of them, so
// reporting degraded there would roll back a working release.
func TestReadinessProbeStaysQuietWhileTheDirectPathIsUp(t *testing.T) {
	atomic := config.NewAtomicConfig(&config.Config{
		ActiveSite:     "cline-pass",
		AnthropicFirst: config.AnthropicFirstConfig{Enabled: true},
	}, "")
	up := stubAvailability{"cline-pass": true, "opencode-go": true, "commandcode": true, "workbuddy": true, "opencode-zen": true, "aws-bedrock": true, "openrouter": true}
	if _, degraded := readinessProbe(atomic, up, func() bool { return false })(); degraded {
		t.Fatal("readiness judged an instance whose direct path is serving")
	}
}

// A gate that is down with nothing behind it is an instance that cannot serve.
// Enabling the passthrough used to make this endpoint stop looking entirely, so
// exactly that release passed the deploy gate.
func TestReadinessProbeJudgesADownDirectPathAgainstTheFallback(t *testing.T) {
	all := stubAvailability{"cline-pass": true, "opencode-go": true, "commandcode": true, "workbuddy": true, "opencode-zen": true, "aws-bedrock": true, "openrouter": true}
	for _, tc := range []struct {
		name        string
		exhausted   stubAvailability
		directDown  bool
		wantDegrade bool
	}{
		{"direct up, nothing else", all, false, false},
		{"direct down, nothing else", all, true, true},
		{"direct down, a platform can serve", stubAvailability{"cline-pass": false}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			atomic := config.NewAtomicConfig(&config.Config{AnthropicFirst: config.AnthropicFirstConfig{Enabled: true}}, "")
			_, degraded := readinessProbe(atomic, tc.exhausted, func() bool { return tc.directDown })()
			if degraded != tc.wantDegrade {
				t.Fatalf("degraded = %v, want %v", degraded, tc.wantDegrade)
			}
		})
	}
}
