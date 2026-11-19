package provider

import (
	"context"
	"time"

	"github.com/routatic/proxy/internal/quota"
)

// keyHealthProbeInterval is how often each configured credential's
// subscription endpoint is re-read. It is short compared to a plan actually
// lapsing and long compared to any reasonable account-endpoint rate limit.
const keyHealthProbeInterval = 10 * time.Minute

// StartHealthLoop gates key rotation on the subscription probe until ctx is
// cancelled. It is started by the server wiring, not the constructor, so tests
// that only need the provider's request paths never touch the network.
func (p *ClinePassProvider) StartHealthLoop(ctx context.Context) {
	if p.health == nil {
		return
	}
	go func() {
		p.probeKeyHealth(ctx)
		ticker := time.NewTicker(keyHealthProbeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.probeKeyHealth(ctx)
			}
		}
	}()
}

// probeKeyHealth reads each configured credential's subscription endpoint once
// and records the verdict with the rotation gate. The endpoint is the same one
// the Quota tab reads - a 404 there is "no plan history found for user", which
// is what a dead entitlement looks like from the account side.
func (p *ClinePassProvider) probeKeyHealth(ctx context.Context) {
	cfg := p.atomic.Get()
	if cfg == nil {
		return
	}
	keys := cfg.ClinePass.EffectiveAPIKeys()
	if len(keys) == 0 {
		return
	}
	endpoint, err := quota.ClinePassUsageURL(cfg.ClinePass.BaseURL)
	if err != nil {
		// An unusable base_url disables the panel's fetch too; the pool's own
		// quarantine still bounds bad-key damage in the meantime.
		return
	}
	for _, key := range keys {
		if ctx.Err() != nil {
			return
		}
		prev := p.health.Excluded(key)
		_, ferr := quota.FetchClinePass(ctx, nil, endpoint, key)
		p.health.Record(key, ferr)
		if now := p.health.Excluded(key); now != prev {
			logKeyExclusion(key, now, ferr)
		}
	}
}
