package metrics

import (
	"context"
	"io"
	"mime"
	"net/http"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/core"
)

const MinGenerationDurationMs int64 = 100

// GenerationSample belongs to one source stream, not to the billing counters.
type GenerationSample struct {
	FirstOutputMs int64 `json:"first_output_ms"`
	DurationMs    int64 `json:"duration_ms"`
	OutputTokens  int64 `json:"output_tokens"`
}

func (s *GenerationSample) Valid() bool {
	return s != nil && s.FirstOutputMs >= 0 && s.DurationMs >= 0 && s.OutputTokens >= 0 && s.OutputTokens <= 1<<53-1
}

func (s *GenerationSample) TokensPerSecond() *float64 {
	if !s.Valid() || s.DurationMs < MinGenerationDurationMs {
		return nil
	}
	rate := float64(s.OutputTokens) * 1000 / float64(s.DurationMs)
	return &rate
}

type generationKey struct{}

// GenerationObservation is attempt-scoped. Its body reads happen before provider
// peeking/capture, while the handler reads the sample only after forwarding ends.
type GenerationObservation struct {
	mu     sync.Mutex
	start  time.Time
	format core.WireFormat
	sample *GenerationSample
}

func WithGenerationObservation(ctx context.Context, start time.Time, format core.WireFormat) (context.Context, *GenerationObservation) {
	o := &GenerationObservation{start: start, format: format}
	return context.WithValue(ctx, generationKey{}, o), o
}

func (o *GenerationObservation) Sample() *GenerationSample {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.sample == nil {
		return nil
	}
	sample := *o.sample
	return &sample
}

// ObserveGenerationResponse must run at the raw HTTP body boundary, before any
// provider wrapper. It never replaces the stream's bytes, errors or Close.
func ObserveGenerationResponse(ctx context.Context, response *http.Response) {
	o, _ := ctx.Value(generationKey{}).(*GenerationObservation)
	if o == nil {
		return
	}
	o.mu.Lock()
	o.sample = nil
	o.mu.Unlock()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	encoding := response.Header.Get("Content-Encoding")
	if err != nil || mediaType != "text/event-stream" || (encoding != "" && encoding != "identity") || o.format == core.WireFormatGemini {
		return
	}
	response.Body = &generationBody{ReadCloser: response.Body, observation: o}
}

type generationBody struct {
	io.ReadCloser
	observation *GenerationObservation
	line, data  []byte
	event       string
	frameBytes  int
	skipLF      bool
	invalid     bool
	terminal    bool
	firstOutput time.Time
	output      *int64
}

func (b *generationBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	at := time.Now()
	b.observation.mu.Lock()
	defer b.observation.mu.Unlock()
	if !b.invalid && !b.terminal {
		b.observe(p[:n], at)
		if err != nil && err != io.EOF && !b.terminal {
			b.invalid = true
		}
	}
	return n, err
}
