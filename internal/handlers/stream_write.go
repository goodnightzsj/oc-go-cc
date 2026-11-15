package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Bound each network write, not the lifetime of a long-running stream.
func withStreamWriteDeadline(w http.ResponseWriter, timeout time.Duration, write func() error) error {
	if timeout <= 0 {
		timeout = keepaliveWriteTimeout
	}
	c := http.NewResponseController(w)
	err := c.SetWriteDeadline(time.Now().Add(timeout))
	// In-memory writers (including httptest) have no network deadline.
	if errors.Is(err, http.ErrNotSupported) {
		return write()
	}
	if err != nil {
		return fmt.Errorf("set stream write deadline: %w", err)
	}
	if err := write(); err != nil {
		return err // Leave the deadline in place after a failed delivery.
	}
	if err := c.SetWriteDeadline(time.Time{}); err != nil {
		return fmt.Errorf("clear stream write deadline: %w", err)
	}
	return nil
}

// Caller holds mu. A broken downstream must stop source reads, not trigger fallback.
func (w *responseWriter) writeIO(timeout time.Duration, write func() error) error {
	if w.writeErr != nil {
		return w.writeErr
	}
	if err := withStreamWriteDeadline(w.ResponseWriter, timeout, write); err != nil {
		w.writeErr = err
		if w.cancelWrites != nil {
			w.cancelWrites()
		}
		return err
	}
	return nil
}
