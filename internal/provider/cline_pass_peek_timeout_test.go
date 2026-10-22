package provider

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/transformer"
)

// gatedBody answers reads from a script and blocks afterwards until released.
// The block is what a stalled upstream looks like to the peek.
type gatedBody struct {
	mu      sync.Mutex
	script  []string
	release chan struct{}
	reads   int
	// ctx is the read's bound, mirroring http.Request.Context on a real body.
	ctx context.Context
}

func (g *gatedBody) Read(p []byte) (int, error) {
	g.mu.Lock()
	if len(g.script) > 0 {
		next := g.script[0]
		g.script = g.script[1:]
		g.reads++
		g.mu.Unlock()
		return copy(p, next), nil
	}
	g.mu.Unlock()

	select {
	case <-g.release:
		return 0, io.EOF
	case <-g.ctx.Done():
		// A real http.Response.Body is bound to the request context and fails
		// this way; the fake must too, or the test would be exercising a stall
		// the production path cannot produce.
		return 0, g.ctx.Err()
	}
}

func (g *gatedBody) Close() error { return nil }

// observerFor wraps g the way the real send path does, so the peek sees the
// same interface it sees in production.
func observerFor(r io.ReadCloser, target string) io.ReadCloser {
	return newClineChannelBody(r, target, "req-test")
}

// A peek that never sees a model must stop when the caller's context does.
// Before this, the peek was the one read in the proxy no idle timeout covered:
// the watchdog starts on the body handed back, which does not exist until the
// peek returns.
func TestPeekStopsWhenContextExpires(t *testing.T) {
	const stall = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	peekCtx, cancelPeek := context.WithCancel(ctx)
	defer cancelPeek()

	body := &gatedBody{
		// One frame that does not name a model, then silence.
		script:  []string{"data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"},
		release: make(chan struct{}),
		ctx:     peekCtx,
	}
	defer close(body.release)

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		_, _, _, _ = peekServedModel(peekCtx, cancelPeek, observerFor(body, ""), maxServedModelPeekBytes, stall)
		done <- time.Since(start)
	}()

	select {
	case took := <-done:
		if took > time.Second {
			t.Fatalf("peek returned after %v; the context should have bounded it", took)
		}
		t.Logf("peek bounded in %v", took)
	case <-time.After(2 * time.Second):
		t.Fatal("peek never returned: it is not bound by the context")
	}
}

// A stalled peek must produce the sentinel the handler already treats as "try
// the next model". Returning a replayed body with no error looked like a clean
// short stream, so a stalled gateway produced a truncated answer instead of a
// fallback.
func TestPeekIdleExpiryReportsStreamIdle(t *testing.T) {
	stall := 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	peekCtx, cancelPeek := context.WithCancel(ctx)
	defer cancelPeek()

	body := &gatedBody{
		script:  []string{"data: {\"choices\":[{\"delta\":{}}]}\n\n"},
		release: make(chan struct{}),
		ctx:     peekCtx,
	}
	defer close(body.release)

	peeked, _, _, err := peekServedModel(peekCtx, cancelPeek, observerFor(body, ""), maxServedModelPeekBytes, stall)
	if !errors.Is(err, transformer.ErrStreamIdle) {
		t.Fatalf("peek error = %v, want ErrStreamIdle so the handler falls through to the next model", err)
	}
	if peeked != nil {
		t.Error("a failed peek must not hand back a body that reads as a complete stream")
	}
}

// The bytes the peek consumed must still reach the caller, in order, even when
// it gave up without finding a model.
func TestPeekReplaysWhatItReadWhenItGivesUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	peekCtx, cancelPeek := context.WithCancel(ctx)
	defer cancelPeek()

	// A finite body: the peek must give up on its own once the script ends,
	// because nothing here ever names a model.
	body := &finiteBody{chunks: []string{"alpha\n", "beta\n"}, ctx: peekCtx}

	// The idle bound must outlast the script, or the watchdog would cancel the
	// context before the replay is read - which is the correct behaviour for a
	// stalled stream, but not what this test is about.
	peeked, model, arrived, peekErr := peekServedModel(peekCtx, cancelPeek, observerFor(body, ""), maxServedModelPeekBytes, 5*time.Second)
	if peekErr != nil {
		t.Fatalf("peek returned an error: %v", peekErr)
	}
	if model != "" {
		t.Fatalf("model = %q, want empty: no frame named one", model)
	}
	if !arrived {
		t.Fatal("first byte arrived but was not reported")
	}
	replayed, err := io.ReadAll(peeked)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(replayed), "alpha\n") {
		t.Fatalf("consumed bytes were not replayed: %q", replayed)
	}
}

// The first-byte stamp belongs to the first chunk that arrived, not to the
// moment the peek finished. Here the naming frame arrives well after the first
// chunk, so the two moments are far apart and the difference is visible.
func TestFirstByteStampedAtFirstChunkNotAtPeekEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	peekCtx, cancelPeek := context.WithCancel(ctx)
	defer cancelPeek()

	body := &gatedBody{
		script: []string{
			"data: {\"choices\":[{\"delta\":{}}]}\n\n", // heartbeat, no model
			"data: {\"model\":\"real/model\"}\n\n",     // names the model
		},
		release: make(chan struct{}),
		ctx:     peekCtx,
	}
	defer close(body.release)

	start := time.Now()
	peeked, model, arrived, peekErr := peekServedModel(peekCtx, cancelPeek, observerFor(&slowSecondRead{gatedBody: body, delay: 80 * time.Millisecond}, ""), maxServedModelPeekBytes, 5*time.Second)
	if peekErr != nil {
		t.Fatalf("peek returned an error: %v", peekErr)
	}
	if !arrived {
		t.Fatal("no first byte reported")
	}
	if model != "real/model" {
		t.Fatalf("model = %q, want real/model", model)
	}
	stamped := firstByteOf(peeked)
	if stamped.IsZero() {
		t.Fatal("first byte never stamped")
	}
	latency := stamped.Sub(start)
	total := time.Since(start)
	t.Logf("stamped after %v, peek returned after %v", latency, total)
	if latency > total-40*time.Millisecond {
		t.Errorf("first byte stamped at %v of a %v peek - it was taken at the end, not at the first chunk", latency, total)
	}
}

// firstByteOf reads the stamp through the same interface the handler uses.
func firstByteOf(body io.ReadCloser) time.Time {
	if reporter, ok := body.(interface{ FirstByteAt() time.Time }); ok {
		return reporter.FirstByteAt()
	}
	return time.Time{}
}

// slowSecondRead delays the second Read and everything after it.
type slowSecondRead struct {
	*gatedBody
	delay time.Duration
	n     int
}

func (s *slowSecondRead) Read(p []byte) (int, error) {
	s.n++
	if s.n > 1 {
		time.Sleep(s.delay)
	}
	return s.gatedBody.Read(p)
}

// finiteBody serves a fixed script then EOF.
type finiteBody struct {
	chunks []string
	ctx    context.Context
}

func (f *finiteBody) Read(p []byte) (int, error) {
	if len(f.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, f.chunks[0])
	f.chunks = f.chunks[1:]
	return n, nil
}

func (f *finiteBody) Close() error { return nil }
