package history

// ValidGenerationSample also checks the sample against its owning request;
// subtracting avoids overflow when reading externally supplied durations.
func (r RequestRecord) ValidGenerationSample() bool {
	s := r.GenerationSample
	return r.Streaming && r.Success && !r.UsageUnobserved && s.Valid() &&
		s.DurationMs <= r.Duration.Milliseconds() && s.FirstOutputMs <= r.Duration.Milliseconds()-s.DurationMs
}

func (r RequestRecord) GenerationTokensPerSecond() *float64 {
	if !r.DetailsKnown || !r.ValidGenerationSample() {
		return nil
	}
	return r.GenerationSample.TokensPerSecond()
}

// ContentTokensPerSecond is an observed post-content rate, not pure decoding
// speed: upstream buffering and unstreamed reasoning can shorten the interval.
func (r RequestRecord) ContentTokensPerSecond() *float64 {
	if r.MessageEndMs == nil || *r.MessageEndMs > r.Duration.Milliseconds() {
		return nil
	}
	return r.streamRate(r.FirstContentMs, *r.MessageEndMs)
}

func (r RequestRecord) EventTokensPerSecond() *float64 {
	return r.streamRate(r.FirstEventMs, r.Duration.Milliseconds())
}

func (r RequestRecord) streamRate(start *int64, end int64) *float64 {
	if !r.Streaming || !r.Success || !r.DetailsKnown || r.OutputTokens <= 0 || start == nil || *start < 0 || end <= *start {
		return nil
	}
	rate := float64(r.OutputTokens) * 1000 / float64(end-*start)
	return &rate
}
