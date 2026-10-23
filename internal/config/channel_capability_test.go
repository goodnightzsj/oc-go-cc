package config

import (
	"strings"
	"testing"
)

// A capability the gateway does not recognise is refused with
// stream_initialization_failed, and that refusal happens on the live request -
// after the pin has been chosen and the key spent. Nothing downstream can
// recover from it, so the value has to be rejected before the config is
// accepted.
func TestUnknownChannelCapabilityIsRejectedAtLoad(t *testing.T) {
	for _, capability := range []string{
		"definitely-not-real",
		"tools",            // measured rejected by both models
		"function-calling", // measured rejected by both models
		"Vision",           // the gateway is case-sensitive
		"vision ",          // trailing space is not trimmed into validity
	} {
		err := ValidateChannelCapabilities(map[string][]string{
			"deepseek-v4.1-flash-1017": {capability},
		})
		if err == nil {
			t.Errorf("capability %q was accepted; the gateway refuses it at request time", capability)
		} else if !strings.Contains(err.Error(), capability) {
			t.Errorf("error for %q does not name the offending value: %v", capability, err)
		}
	}
}

// The four values the gateway accepted when measured must pass, or the feature
// cannot be configured at all.
func TestKnownChannelCapabilitiesAreAccepted(t *testing.T) {
	for _, capability := range []string{
		"vision",
		"implicit-caching",
		"reasoning",
		"structured-output",
	} {
		if err := ValidateChannelCapabilities(map[string][]string{
			"deepseek-v4.1-flash-1017": {capability},
		}); err != nil {
			t.Errorf("capability %q was rejected: %v", capability, err)
		}
	}
}

// An unset filter is the shape every existing deployment sends. It must stay
// valid, so adding this feature cannot break a config that never mentions it.
func TestNoChannelCapabilitiesIsValid(t *testing.T) {
	if err := ValidateChannelCapabilities(nil); err != nil {
		t.Errorf("nil map rejected: %v", err)
	}
	if err := ValidateChannelCapabilities(map[string][]string{}); err != nil {
		t.Errorf("empty map rejected: %v", err)
	}
	if err := ValidateChannelCapabilities(map[string][]string{"m": {}}); err != nil {
		t.Errorf("empty slice rejected: %v", err)
	}
}

// The filter is per model because it is a hard failure, not a hint: vision is
// accepted for one deepseek model and refused for the other. Validation must
// therefore check every model's list, not just the first.
func TestEveryModelsCapabilityListIsChecked(t *testing.T) {
	err := ValidateChannelCapabilities(map[string][]string{
		"deepseek-v4.1-flash-1017": {"vision"}, // valid
		"deepseek-v4-pro":          {"image-input"}, // rejected by the gateway
	})
	if err == nil {
		t.Fatal("an invalid capability on the second model was accepted")
	}
	if !strings.Contains(err.Error(), "image-input") {
		t.Errorf("error does not name the offending capability: %v", err)
	}
}
