package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
)

type clineChannelOptions struct {
	Only []string `json:"only"`
}

// clineChannelBody observes routing without changing bytes, buffering the full
// response or introducing a goroutine. Metadata can arrive after finish_reason.
type clineChannelBody struct {
	io.ReadCloser
	mu                                   sync.Mutex
	line                                 []byte
	dropping, done, reported, incomplete bool
	target, actual, requestID            string
}

func newClineChannelBody(body io.ReadCloser, target, requestID string) *clineChannelBody {
	return &clineChannelBody{ReadCloser: body, target: target, requestID: requestID}
}

func (b *clineChannelBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	for rest := p[:n]; len(rest) > 0; {
		piece, after, newline := bytes.Cut(rest, []byte{'\n'})
		// Match the existing SSE parser's 1 MiB line ceiling. An oversized
		// line is unobservable, never a reason to corrupt or truncate the stream.
		if len(b.line)+len(piece) > 1<<20 {
			b.line = nil
			b.dropping = true
			b.incomplete = true
		}
		if !b.dropping {
			b.line = append(b.line, piece...)
		}
		if newline {
			if !b.dropping {
				b.observe(b.line)
			}
			b.line = b.line[:0]
			b.dropping = false
		}
		rest = after
	}
	if err != nil {
		b.report()
	}
	return n, err
}

func (b *clineChannelBody) Close() error {
	err := b.ReadCloser.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.report()
	return err
}

type clineRoutingMetadata struct {
	ProviderMetadata struct {
		Gateway struct {
			Routing struct {
				FinalProvider string `json:"finalProvider"`
			} `json:"routing"`
		} `json:"gateway"`
	} `json:"provider_metadata"`
}

func (b *clineChannelBody) observe(line []byte) {
	data, ok := bytes.CutPrefix(line, []byte("data:"))
	if !ok {
		return
	}
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("[DONE]")) {
		b.done = true
		return
	}
	var chunk struct {
		clineRoutingMetadata
		Choices []struct {
			Delta   clineRoutingMetadata `json:"delta"`
			Message clineRoutingMetadata `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(data, &chunk) != nil {
		return // Observation is advisory; the existing stream parser owns errors.
	}
	remember := func(m clineRoutingMetadata) {
		if actual := m.ProviderMetadata.Gateway.Routing.FinalProvider; actual != "" {
			b.actual = actual
		}
	}
	remember(chunk.clineRoutingMetadata)
	for _, choice := range chunk.Choices {
		remember(choice.Message)
		remember(choice.Delta)
	}
}

func (b *clineChannelBody) report() {
	if b.reported {
		return
	}
	b.reported = true
	status := "unverified"
	if b.done && !b.incomplete && b.actual != "" {
		status = "mismatch"
		actual, target := b.actual, b.target
		if actual == "z-ai" {
			actual = "zai"
		}
		if target == "z-ai" {
			target = "zai"
		}
		if actual == target {
			status = "matched"
		}
	}
	args := []any{"request_id", b.requestID, "requested_provider", b.target, "actual_provider", b.actual, "adherence", status}
	if status == "matched" {
		slog.Info("cline-pass channel pin observation (match is not proof of enforcement)", args...)
	} else {
		slog.Warn("cline-pass channel pin not confirmed", args...)
	}
}
