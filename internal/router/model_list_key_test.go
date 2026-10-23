package router

import (
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
)

// Only the platform whose roster belongs to an account gets a credential.
// Sending one to the public lists would hand a key to an endpoint that never
// asked for it, and the mistake would be invisible: those endpoints ignore the
// header and still answer 200.
func TestModelListKeyOnlyForAccountScopedRosters(t *testing.T) {
	cfg := &config.Config{
		APIKey:      "global-key",
		CommandCode: config.CommandCodeConfig{APIKey: "cc-key"},
		ClinePass:   config.ClinePassConfig{APIKey: "cp-key"},
		WorkBuddy:   config.WorkBuddyConfig{APIKey: "wb-key"},
	}

	if got := modelListKey(cfg, site.WorkBuddy); got != "wb-key" {
		t.Errorf("workbuddy key = %q, want wb-key", got)
	}
	for _, provider := range []string{site.CommandCode, site.ClinePass, site.OpenCodeGo} {
		if got := modelListKey(cfg, provider); got != "" {
			t.Errorf("%s key = %q, want none (its roster is public)", provider, got)
		}
	}
}

// A platform with no credential at all must not panic or invent one.
func TestModelListKeyWithoutConfiguration(t *testing.T) {
	if got := modelListKey(nil, site.WorkBuddy); got != "" {
		t.Errorf("nil config returned %q", got)
	}
	if got := modelListKey(&config.Config{}, site.WorkBuddy); got != "" {
		t.Errorf("unconfigured workbuddy returned %q", got)
	}
}

// A pool with several keys presents the first, matching what the send path
// does for its own first attempt.
func TestModelListKeyUsesTheFirstOfAPool(t *testing.T) {
	cfg := &config.Config{WorkBuddy: config.WorkBuddyConfig{APIKeys: []string{"first", "second"}}}
	if got := modelListKey(cfg, site.WorkBuddy); got != "first" {
		t.Errorf("got %q, want first", got)
	}
}
