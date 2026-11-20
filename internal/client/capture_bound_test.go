package client

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// A captured body is debug data, not the response. Without a ceiling every
// streamed answer became a full in-memory copy in the capture queue; without
// draining the remainder, stopping the read blocks the tee and stalls the very
// response being captured. Neither half is visible from a small fixture.
func TestCaptureBodyBoundsAndDrains(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), maxCapturedBody+1024)
	captured := make(chan []byte, 1)
	body := CaptureBody(io.NopCloser(bytes.NewReader(payload)), func(data []byte) {
		captured <- data
	})

	done := make(chan []byte, 1)
	go func() {
		got, _ := io.ReadAll(body)
		done <- got
	}()
	var forwarded []byte
	select {
	case forwarded = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reading the wrapped body blocked: the capture side stopped draining")
	}
	if !bytes.Equal(forwarded, payload) {
		t.Fatalf("forwarded %d bytes, want the full %d", len(forwarded), len(payload))
	}
	_ = body.Close()

	select {
	case data := <-captured:
		if want := maxCapturedBody + len(captureTruncatedMarker); len(data) != want {
			t.Fatalf("captured %d bytes, want %d", len(data), want)
		}
		if !bytes.HasSuffix(data, []byte(captureTruncatedMarker)) {
			t.Fatal("a truncated capture must end with the marker")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("capture callback never ran")
	}
}

// Exactly at the limit there is nothing to truncate.
func TestCaptureBodyKeepsABodyAtTheLimit(t *testing.T) {
	payload := bytes.Repeat([]byte("y"), maxCapturedBody)
	captured := make(chan []byte, 1)
	body := CaptureBody(io.NopCloser(bytes.NewReader(payload)), func(data []byte) { captured <- data })
	if _, err := io.ReadAll(body); err != nil {
		t.Fatalf("read: %v", err)
	}
	_ = body.Close()
	select {
	case data := <-captured:
		if len(data) != maxCapturedBody || bytes.Contains(data, []byte(captureTruncatedMarker)) {
			t.Fatalf("captured %d bytes with marker=%v, want the body unchanged", len(data), bytes.Contains(data, []byte(captureTruncatedMarker)))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("capture callback never ran")
	}
}
