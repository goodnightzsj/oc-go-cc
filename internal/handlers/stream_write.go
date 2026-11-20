package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/routatic/proxy/internal/core"
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
		var source *core.ResponseError
		// Keep an already observed source failure when its delivery also fails.
		if w.streamErr != nil && !errors.As(err, &source) {
			err = errors.Join(&core.ResponseError{Err: w.streamErr}, err)
		}
		w.writeErr = err
		if !errors.As(err, &source) && w.cancelWrites != nil {
			w.cancelWrites()
		}
		return err
	}
	return nil
}

// Caller holds mu. Local failures bypass the source decoder and end any
// already-forwarded partial frame before emitting their own terminal event.
func (w *responseWriter) writeErrorEvent(data []byte) error {
	// An Anthropic error event is terminal. When the upstream already sent one,
	// the bytes are on the wire and a second event both violates that and hides
	// the real cause from a client that surfaces the last error it saw.
	if w.sourceErrored {
		return nil
	}
	w.sourceErrored = true
	return w.writeIO(w.writeTimeout, func() error {
		if !w.wroteHeader {
			w.wroteHeader = true
			w.ResponseWriter.WriteHeader(http.StatusOK)
		}
		// The error event is caller-visible, so it counts as payload: nothing
		// can be replaced by a fallback after it.
		w.payloadWritten = true
		frame := fmt.Sprintf("\n\nevent: error\ndata: %s\n\n", data)
		n, err := w.ResponseWriter.Write([]byte(frame))
		if err != nil {
			return err
		}
		if n != len(frame) {
			return io.ErrShortWrite
		}
		return http.NewResponseController(w.ResponseWriter).Flush()
	})
}
