// Package debug provides request/response capture functionality for debugging.
package debug

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// CaptureLogger handles async capture of request/response data for debugging.
// It uses a buffered channel and background worker to avoid blocking the main request flow.
type CaptureLogger struct {
	storage   *Storage
	enabled   bool
	entryChan chan CaptureEntry
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewCaptureLogger creates a new async capture logger.
// Returns nil if capture is not enabled or storage is nil.
// The logger starts a background worker goroutine that processes capture entries.
func NewCaptureLogger(storage *Storage, enabled bool) *CaptureLogger {
	if !enabled || storage == nil {
		return nil
	}

	cl := &CaptureLogger{
		storage:   storage,
		enabled:   enabled,
		entryChan: make(chan CaptureEntry, 100),
	}

	// Start background worker
	cl.wg.Add(1)
	go cl.worker()

	return cl
}

// worker is the background goroutine that processes capture entries.
// It reads from entryChan and writes to storage until the channel is closed.
func (c *CaptureLogger) worker() {
	defer c.wg.Done()

	for entry := range c.entryChan {
		if err := c.storage.WriteEntry(entry); err != nil {
			slog.Warn("failed to write capture entry",
				"error", err,
				"request_id", entry.RequestID,
				"phase", entry.Phase,
			)
		}
	}
}

// capture builds an entry for the given phase and hands it to the background
// worker. All Capture* methods differ only by phase.
func (c *CaptureLogger) capture(phase, requestID, provider string, data []byte) {
	if c == nil || !c.enabled {
		return
	}

	c.sendEntry(CaptureEntry{
		Timestamp: time.Now(),
		Phase:     phase,
		Provider:  provider,
		RequestID: requestID,
		Data:      redactIfNeeded(data, c.storage.config.RedactAPIKeys),
	})
}

// CaptureOriginal captures the original incoming request, before any transform.
func (c *CaptureLogger) CaptureOriginal(requestID string, data []byte) {
	c.capture(PhaseOriginal, requestID, "", data)
}

// CaptureNormalized captures the request after normalization to the internal format.
func (c *CaptureLogger) CaptureNormalized(requestID, provider string, data []byte) {
	c.capture(PhaseNormalized, requestID, provider, data)
}

// CaptureUpstreamRequest captures the request as sent to the upstream provider.
func (c *CaptureLogger) CaptureUpstreamRequest(requestID, provider string, data []byte) {
	c.capture(PhaseUpstreamRequest, requestID, provider, data)
}

// CaptureUpstreamResponse captures the raw upstream response, before transforming
// it back to the Anthropic format.
func (c *CaptureLogger) CaptureUpstreamResponse(requestID, provider string, data []byte) {
	c.capture(PhaseUpstreamResponse, requestID, provider, data)
}

// sendEntry sends an entry to the background worker via the buffered channel.
// It uses a non-blocking select with default to avoid blocking if the channel is full.
func (c *CaptureLogger) sendEntry(entry CaptureEntry) {
	select {
	case c.entryChan <- entry:
		// Entry queued successfully
	default:
		// Channel is full, log and drop the entry to avoid blocking
		slog.Warn("capture channel full, dropping entry",
			"request_id", entry.RequestID,
			"phase", entry.Phase,
		)
	}
}

// redactIfNeeded applies RedactSensitive to data if redaction is enabled.
// Returns the original data as json.RawMessage if redaction is disabled.
// Returns the redacted data as json.RawMessage if redaction is enabled.
func redactIfNeeded(data []byte, redactEnabled bool) json.RawMessage {
	if !redactEnabled {
		return json.RawMessage(data)
	}

	redacted := RedactSensitive(data)
	return json.RawMessage(redacted)
}

// Close shuts down the capture logger.
// It closes the entry channel and waits for the background worker to finish.
// Any entries still in the channel will be processed before Close returns.
// Safe to call multiple times - only the first call will have effect.
func (c *CaptureLogger) Close() error {
	if c == nil {
		return nil
	}

	c.closeOnce.Do(func() {
		// Close the channel to signal the worker to exit
		close(c.entryChan)

		// Wait for the worker to finish processing remaining entries
		c.wg.Wait()
	})

	return nil
}
