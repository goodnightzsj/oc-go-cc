package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"time"
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
	// upstreamModel is the model the gateway says it actually ran, which is not
	// always the id we sent. The gateway rewrites some request ids onto a
	// virtual model (`vmc/...-contributor-fallbacks`) and routes those through
	// its own fallback chain, so the id in the request stops describing what
	// produced the answer. Observing it is the only way to report that without
	// changing the response the client already received.
	upstreamModel string
	// firstByteAt is when the upstream's first byte arrived, set by the peek.
	// Zero means it was not measured.
	firstByteAt time.Time
	// servedModel is the OpenAI chunk's `model` field as the gateway filled it,
	// which is available before any content is written. UpstreamModel is the
	// same fact from the routing block at the end of the stream; ServedModel()
	// prefers this one because it is the only one a response can carry.
	servedModel string
	// keyHint labels the credential this attempt used, for the history record.
	keyHint string
}

func newClineChannelBody(body io.ReadCloser, target, requestID string) *clineChannelBody {
	return &clineChannelBody{ReadCloser: body, target: target, requestID: requestID}
}

// KeyHint reports the masked label of the credential that carried this attempt.
func (b *clineChannelBody) KeyHint() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.keyHint
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

// UpstreamChannel reports the channel the gateway named for this request, or ""
// while the metadata frame has not arrived yet. It is the value the per-request
// log line and the stored record both use, read from one place.
func (b *clineChannelBody) UpstreamChannel() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.actual
}

// ServedModel reports the model from the first upstream chunk, which is
// available before any content is written. Empty until that frame arrives.
//
// This is the value a response can honestly carry: message_start has to be
// emitted before the body is forwarded, and this is the one model field
// knowable at that moment. See ServedModelReader.
// MarkFirstByte stamps the moment the upstream's first byte was observed. The
// peek calls it once; a second call is ignored so a later peek cannot move the
// timestamp forward.
func (b *clineChannelBody) MarkFirstByte() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.firstByteAt.IsZero() {
		b.firstByteAt = time.Now()
	}
}

// FirstByteAt reports when the upstream's first byte arrived, or the zero time
// when it was never observed.
func (b *clineChannelBody) FirstByteAt() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.firstByteAt
}

func (b *clineChannelBody) ServedModel() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.servedModel
}

// UpstreamModel reports the model the gateway says it actually ran, or "" while
// the metadata frame has not arrived. See UpstreamModelReader.
func (b *clineChannelBody) UpstreamModel() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.upstreamModel
}

// clineRoutingMetadata is the routing block the gateway attaches to a response.
// It is shared by the per-request observer and the pin probe because both read
// the same block and must agree on what a field means.
//
// Three of the four fields are read even though a pinned request usually
// reports nothing useful in them, and that is deliberate:
//
//   - FinalProvider is the channel that served the request.
//   - ResolvedProvider is the same fact under the name the direct pipeline
//     (OpenRouter) uses. Reading only finalProvider makes a direct-pipeline
//     answer look like no metadata at all.
//   - FallbacksAvailable is the channel list the gateway considered. It is the
//     only place an *accepted* request names other channels, so it is what makes
//     a pinnable account distinguishable from a private-route one without
//     relying on a rejection.
//   - CanonicalSlug names the upstream model, which is what ties a response to
//     the model that produced it when the gateway rewrites the id.
type clineRoutingMetadata struct {
	ProviderMetadata struct {
		Gateway struct {
			Routing struct {
				FinalProvider      string   `json:"finalProvider"`
				ResolvedProvider   string   `json:"resolvedProvider"`
				CanonicalSlug      string   `json:"canonicalSlug"`
				FallbacksAvailable []string `json:"fallbacksAvailable"`
			} `json:"routing"`
		} `json:"gateway"`
	} `json:"provider_metadata"`
}

// channel returns the channel that served the request, under whichever name
// this pipeline publishes it. Both spellings mean the same thing, and only one
// is populated per response.
func (m clineRoutingMetadata) channel() string {
	routing := m.ProviderMetadata.Gateway.Routing
	if routing.FinalProvider != "" {
		return routing.FinalProvider
	}
	return routing.ResolvedProvider
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
		// Model is the OpenAI chunk's own model field, which the gateway fills
		// on every frame with the id it actually ran. It is the authoritative
		// answer for "what produced this", and it is available on the FIRST
		// frame - unlike canonicalSlug, which arrives in the routing block at
		// the end. Both are recorded because they answer from different
		// moments: Model is what the caller can be told before any content is
		// written, canonicalSlug is what survives to the stored record.
		Model   string `json:"model"`
		Choices []struct {
			Delta   clineRoutingMetadata `json:"delta"`
			Message clineRoutingMetadata `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(data, &chunk) != nil {
		return // Observation is advisory; the existing stream parser owns errors.
	}
	// The first frame wins: a later frame repeating the same value changes
	// nothing, and one that disagrees would be reporting a second attempt.
	if b.servedModel == "" && chunk.Model != "" {
		b.servedModel = chunk.Model
	}
	remember := func(m clineRoutingMetadata) {
		if actual := m.channel(); actual != "" {
			b.actual = actual
		}
		// canonicalSlug names the upstream model. It is set on the same routing
		// block as the channel, so it costs one more assignment here.
		if slug := m.ProviderMetadata.Gateway.Routing.CanonicalSlug; slug != "" {
			b.upstreamModel = slug
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
	// No target means pinning is off. There is then no verdict to give - the
	// request was never pinned - and the observed channel is still recorded by
	// the caller, so reporting one here would only be noise.
	if b.target == "" {
		return
	}
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
