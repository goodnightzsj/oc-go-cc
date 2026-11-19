package provider

import (
	"context"
	"log/slog"
	"time"

	"github.com/routatic/proxy/internal/client"
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
		probed, excluded := p.probeKeyHealth(ctx)
		// One line at start-up answers "is the gate live and what does it see"
		// without turning a ten-minute cadence into log noise.
		slog.Info("cline-pass key health gate started", "provider", client.ProviderClinePass,
			"interval", keyHealthProbeInterval.String(), "keys_probed", probed, "keys_excluded", excluded)
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
// is what a dead entitlement looks like from the account side. It reports how
// many keys it probed and how many the gate now holds out.
func (p *ClinePassProvider) probeKeyHealth(ctx context.Context) (probed, excluded int) {
	cfg := p.atomic.Get()
	if cfg == nil {
		return 0, 0
	}
	keys := cfg.ClinePass.EffectiveAPIKeys()
	if len(keys) == 0 {
		return 0, 0
	}
	endpoint, err := quota.ClinePassUsageURL(cfg.ClinePass.BaseURL)
	if err != nil {
		// An unusable base_url disables the panel's fetch too; the pool's own
		// quarantine still bounds bad-key damage in the meantime.
		return 0, 0
	}
	for _, key := range keys {
		if ctx.Err() != nil {
			return probed, excluded
		}
		prev := p.health.Excluded(key)
		_, ferr := quota.FetchClinePass(ctx, nil, endpoint, key)
		p.health.Record(key, ferr)
		now := p.health.Excluded(key)
		if now != prev {
			logKeyExclusion(key, now, ferr)
		}
		if now {
			excluded++
		}
		probed++
	}
	return probed, excluded
}
