package history

import (
	"testing"
	"time"
)

func TestStreamRatesUseObservedIntervals(t *testing.T) {
	p := func(n int64) *int64 { return &n }
	base := RequestRecord{Streaming: true, Success: true, DetailsKnown: true, OutputTokens: 600, ReasoningTokens: 100, Duration: 16 * time.Second, FirstEventMs: p(1000), FirstContentMs: p(4000), MessageEndMs: p(14000)}
	for _, tc := range []struct {
		name           string
		change         func(*RequestRecord)
		content, event float64
	}{
		{"measured", func(*RequestRecord) {}, 60, 40},
		{"zero is observed", func(r *RequestRecord) { r.FirstEventMs = p(0); r.FirstContentMs = p(0) }, 600.0 / 14, 37.5},
		{"unknown", func(r *RequestRecord) { r.FirstEventMs = nil; r.FirstContentMs = nil }, 0, 0},
		{"failure", func(r *RequestRecord) { r.Success = false }, 0, 0},
		{"nonstream", func(r *RequestRecord) { r.Streaming = false }, 0, 0},
		{"unobserved usage", func(r *RequestRecord) { r.DetailsKnown = false }, 0, 0},
		{"zero output", func(r *RequestRecord) { r.OutputTokens = 0 }, 0, 0},
		{"missing end", func(r *RequestRecord) { r.MessageEndMs = nil }, 0, 40},
		{"same batch", func(r *RequestRecord) { r.FirstContentMs = p(14000) }, 0, 40},
		{"reversed", func(r *RequestRecord) { r.FirstContentMs = p(15000) }, 0, 40},
		{"end outside execution", func(r *RequestRecord) { r.MessageEndMs = p(17000) }, 0, 40},
		{"negative", func(r *RequestRecord) { r.FirstEventMs = p(-1); r.FirstContentMs = p(-1) }, 0, 0},
		{"event at end", func(r *RequestRecord) { r.FirstEventMs = p(16000) }, 60, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.change(&r)
			for _, v := range []struct {
				name string
				got  *float64
				want float64
			}{{"content", r.ContentTokensPerSecond(), tc.content}, {"event", r.EventTokensPerSecond(), tc.event}} {
				if v.want == 0 {
					if v.got != nil {
						t.Errorf("%s=%v, want unknown", v.name, *v.got)
					}
				} else if v.got == nil || *v.got != v.want {
					t.Errorf("%s=%v want %v", v.name, v.got, v.want)
				}
			}
		})
	}
}
