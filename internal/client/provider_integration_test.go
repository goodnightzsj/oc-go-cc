package client

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
)

func TestCommandCodeUsesIndependentTimeouts(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo:  config.OpenCodeGoConfig{TimeoutMs: 999, StreamTimeoutMs: 998, StreamingTimeoutMs: 997},
		CommandCode: config.CommandCodeConfig{TimeoutMs: 1100, StreamTimeoutMs: 2200, StreamingTimeoutMs: 3300},
	}
	c := NewOpenCodeClient(config.NewAtomicConfig(cfg, ""), nil)
	model := config.ModelConfig{Provider: "commandcode", ModelID: "example"}
	if got := c.RequestTimeout(model); got != 1100*time.Millisecond {
		t.Errorf("request timeout = %v", got)
	}
	if got := c.StreamIdleTimeout(model); got != 2200*time.Millisecond {
		t.Errorf("idle timeout = %v", got)
	}
	if got := c.StreamingTimeout(model); got != 3300*time.Millisecond {
		t.Errorf("streaming timeout = %v", got)
	}
}
