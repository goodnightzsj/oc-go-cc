package provider

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// A reader may return bytes and an error together, including a model frame.
type peekFailureBody struct {
	data string
	err  error
	read bool
}

func (b *peekFailureBody) Read(p []byte) (int, error) {
	if b.read {
		return 0, io.EOF
	}
	b.read = true
	return copy(p, b.data), b.err
}
func (b *peekFailureBody) Close() error { return nil }

func TestPeekReadFailure(t *testing.T) {
	const modelFrame = "data: {\"model\":\"real/model\"}\n\n"
	for _, tc := range []struct {
		name, data string
		err        error
	}{
		{"no bytes", "", io.ErrUnexpectedEOF},
		{"heartbeat", ": opened\n\n", io.ErrUnexpectedEOF},
		{"model and error", modelFrame, io.ErrUnexpectedEOF},
		{"clean EOF", modelFrame, io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := &peekFailureBody{data: tc.data, err: tc.err}
			peeked, _, arrived, err := peekServedModel(ctx, cancel, observerFor(body, ""), maxServedModelPeekBytes, time.Second)
			if tc.err != io.EOF {
				if !errors.Is(err, tc.err) || peeked != nil {
					t.Fatalf("peek body=%v error=%v, want nil body and %v", peeked, err, tc.err)
				}
				return
			}
			if err != nil || !arrived {
				t.Fatalf("normal EOF: arrived=%t error=%v", arrived, err)
			}
			replayed, err := io.ReadAll(peeked)
			if err != nil || string(replayed) != tc.data {
				t.Fatalf("replay=%q error=%v", replayed, err)
			}
		})
	}
	for _, deadline := range []bool{false, true} {
		name := "canceled"
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if deadline {
			name = "deadline"
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
		}
		t.Run(name, func(t *testing.T) {
			body := &peekFailureBody{}
			peeked, _, _, err := peekServedModel(ctx, cancel, observerFor(body, ""), maxServedModelPeekBytes, time.Second)
			if !errors.Is(err, ctx.Err()) || peeked != nil || body.read {
				t.Fatalf("canceled peek: body=%v read=%t error=%v", peeked, body.read, err)
			}
		})
	}
}
