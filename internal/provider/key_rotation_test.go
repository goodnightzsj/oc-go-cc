package provider

import (
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestNextAPIKey_RoundRobin(t *testing.T) {
	cfg := &config.Config{
		APIKeys: []string{"key-a", "key-b", "key-c"},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := newBaseProvider(atomicCfg)

	// With 3 keys, iteration 0..5 should cycle key-a, key-b, key-c, key-a, key-b, key-c
	expected := []string{"key-a", "key-b", "key-c", "key-a", "key-b", "key-c"}
	for i, want := range expected {
		if got := c.nextAPIKey(cfg.EffectiveAPIKeys()); got != want {
			t.Errorf("iteration %d: nextAPIKey() = %q, want %q", i, got, want)
		}
	}
}

func TestNextAPIKey_SingleKey(t *testing.T) {
	cfg := &config.Config{APIKey: "single"}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := newBaseProvider(atomicCfg)

	for i := 0; i < 5; i++ {
		if got := c.nextAPIKey(cfg.EffectiveAPIKeys()); got != "single" {
			t.Errorf("iteration %d: nextAPIKey() = %q, want %q", i, got, "single")
		}
	}
}

func TestNextAPIKey_EmptyKeys(t *testing.T) {
	cfg := &config.Config{APIKey: ""}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := newBaseProvider(atomicCfg)

	if got := c.nextAPIKey(cfg.EffectiveAPIKeys()); got != "" {
		t.Errorf("nextAPIKey() = %q, want empty string", got)
	}
}

func TestNextAPIKey_ConcurrentSafety(t *testing.T) {
	cfg := &config.Config{
		APIKeys: []string{"k1", "k2", "k3"},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := newBaseProvider(atomicCfg)

	const goroutines = 3
	const callsPerGoroutine = 100
	results := make(chan string, goroutines*callsPerGoroutine)

	for g := 0; g < goroutines; g++ {
		go func() {
			for i := 0; i < callsPerGoroutine; i++ {
				results <- c.nextAPIKey(cfg.EffectiveAPIKeys())
			}
		}()
	}

	seen := make(map[string]int)
	for i := 0; i < goroutines*callsPerGoroutine; i++ {
		key := <-results
		seen[key]++
	}

	// Each key should be seen exactly (goroutines*callsPerGoroutine)/3 times
	total := goroutines * callsPerGoroutine
	expectedPerKey := total / len(cfg.APIKeys)
	for _, key := range cfg.APIKeys {
		if seen[key] != expectedPerKey {
			t.Errorf("key %q seen %d times, want %d", key, seen[key], expectedPerKey)
		}
	}
}
