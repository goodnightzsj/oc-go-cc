package config

import (
	"encoding/json"
	"testing"
)

func TestClinePassChannelPinValidation(t *testing.T) {
	for _, tc := range []struct {
		name, pin      string
		enabled, valid bool
	}{
		{"default off", "", false, true},
		{"disabled keeps target", "deepseek", false, true},
		{"enabled", "deepseek", true, true},
		{"alias", "z-ai", true, true},
		{"missing target", "", true, false},
		{"blank target", "   ", true, false},
		{"multiple targets", "deepseek,alibaba", true, false},
		{"not a slug", "deepseek/deepseek-v4", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{ClinePass: ClinePassConfig{APIKey: "synthetic", ChannelPinEnabled: tc.enabled, ChannelPin: tc.pin}}
			if err := validate(&cfg); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
			data, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			var restored Config
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			if restored.ClinePass.ChannelPinEnabled != tc.enabled || restored.ClinePass.ChannelPin != tc.pin {
				t.Fatal("channel settings lost during JSON round trip")
			}
		})
	}
}
