package client

import (
	"io"
	"strings"
	"testing"
	"time"
)

// TestCaptureBodyCompletesOnClose guards the pipe-close fix: CaptureBody's async
// capture goroutine must finish once the wrapped body is closed. Before the fix,
// the pipe write end was never closed and the callback never fired.
func TestCaptureBodyCompletesOnClose(t *testing.T) {
	body := io.NopCloser(strings.NewReader("hello-stream"))
	captured := make(chan []byte, 1)

	wrapped := CaptureBody(body, func(data []byte) { captured <- data })

	all, err := io.ReadAll(wrapped)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got, want := string(all), "hello-stream"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if err := wrapped.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case data := <-captured:
		if got, want := string(data), "hello-stream"; got != want {
			t.Fatalf("captured = %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("capture callback never fired; pipe write end was not closed")
	}
}

func TestCaptureBodyCloseWaitsForCallback(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	wrapped := CaptureBody(io.NopCloser(strings.NewReader("")), func([]byte) {
		close(entered)
		<-release
	})
	done := make(chan error, 1)
	go func() { done <- wrapped.Close() }()
	<-entered
	select {
	case err := <-done:
		close(release)
		t.Fatalf("Close returned before capture completed: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
