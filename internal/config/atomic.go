package config

import (
	"log/slog"
	"sync"
	"sync/atomic"
)

// AtomicConfig provides thread-safe access to the configuration with support
// for hot reloading. It uses atomic.Pointer for lock-free reads.
type AtomicConfig struct {
	ptr      atomic.Pointer[Config]
	path     string
	mu       sync.Mutex
	writeMu  sync.Mutex
	onReload []func(*Config)
}

// NewAtomicConfig creates a new AtomicConfig with the given initial config and file path.
func NewAtomicConfig(cfg *Config, path string) *AtomicConfig {
	a := &AtomicConfig{path: path}
	a.ptr.Store(cfg)
	return a
}

// Get returns the current configuration pointer. This is safe for concurrent use.
// Callers must not modify the returned Config.
func (a *AtomicConfig) Get() *Config {
	return a.ptr.Load()
}

// Reload reloads the configuration from disk and atomically swaps it in.
// If the reload fails, the old configuration is preserved and an error is returned.
// On successful reload, all registered callbacks are invoked.
func (a *AtomicConfig) Reload() error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	cfg, err := LoadFromPath(a.path)
	if err != nil {
		return err
	}
	a.applyLoaded(cfg)
	return nil
}

// ApplyLoaded publishes a config previously validated by LoadJSON or
// LoadFromPath. Callers must not mutate it after publishing. GUI saves use this
// after the atomic disk write so a second load cannot fail after persistence.
func (a *AtomicConfig) ApplyLoaded(cfg *Config) {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	a.applyLoaded(cfg)
}

// applyLoaded runs under writeMu so a watcher cannot publish a stale disk read
// after a GUI save, and reload callbacks never run concurrently.
func (a *AtomicConfig) applyLoaded(cfg *Config) {
	old := a.Get()
	// Warn about settings that take effect differently on reload.
	if old != nil {
		if old.Host != cfg.Host || old.Port != cfg.Port {
			slog.Warn("host/port changed but requires server restart to take effect",
				"old_host", old.Host, "new_host", cfg.Host,
				"old_port", old.Port, "new_port", cfg.Port)
		}
		// Timeout changes apply on the next request.
		if old.OpenCodeGo.TimeoutMs != cfg.OpenCodeGo.TimeoutMs ||
			old.OpenCodeGo.StreamingTimeoutMs != cfg.OpenCodeGo.StreamingTimeoutMs ||
			old.OpenCodeZen.TimeoutMs != cfg.OpenCodeZen.TimeoutMs ||
			old.OpenCodeZen.StreamingTimeoutMs != cfg.OpenCodeZen.StreamingTimeoutMs {
			slog.Info("timeout config updated, takes effect immediately",
				"go_timeout_ms", cfg.OpenCodeGo.TimeoutMs,
				"go_streaming_timeout_ms", cfg.OpenCodeGo.StreamingTimeoutMs,
				"zen_timeout_ms", cfg.OpenCodeZen.TimeoutMs,
				"zen_streaming_timeout_ms", cfg.OpenCodeZen.StreamingTimeoutMs)
		}
	}

	// Copy callbacks to avoid holding lock during invocation
	a.mu.Lock()
	callbacks := make([]func(*Config), len(a.onReload))
	copy(callbacks, a.onReload)
	a.mu.Unlock()

	// Invoke callbacks BEFORE swapping — they may mutate cfg (e.g., port override).
	for _, fn := range callbacks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("config reload callback panicked", "panic", r)
				}
			}()
			fn(cfg)
		}()
	}

	// Now cfg is fully prepared — safe for concurrent readers.
	a.ptr.Store(cfg)
}

// OnReload registers a preparation callback invoked before publishing each
// successfully loaded config. Callbacks must not call Reload or ApplyLoaded.
func (a *AtomicConfig) OnReload(fn func(*Config)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onReload = append(a.onReload, fn)
}

// Path returns the config file path being watched.
func (a *AtomicConfig) Path() string {
	return a.path
}
