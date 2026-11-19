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

// keyHealthAsyncProbeFloor is the minimum gap between on-demand probes of the
// same key. An auth rejection asks for an immediate re-judgement; a burst of
// rejections must not become a burst of account-endpoint calls.
const keyHealthAsyncProbeFloor = time.Minute

// keyHealthFirstPassTimeout bounds the synchronous first pass. The gate is
// fail-open, so a request that arrives before the first verdict can take a key
// the probe has not judged yet - on a restart that window produced a real 403.
// Waiting is bounded so a hanging account endpoint cannot delay start-up.
const keyHealthFirstPassTimeout = 3 * time.Second

// StartHealthLoop gates key rotation on the subscription probe until ctx is
// cancelled. It is started by the server wiring, not the constructor, so tests
// that only need the provider's request paths never touch the network. The
// first pass runs synchronously (bounded) before the server starts accepting
// requests; later passes run every interval on a ticker.
func (p *ClinePassProvider) StartHealthLoop(ctx context.Context) {
	if p.health == nil {
		return
	}
	// An auth rejection re-judges the key immediately instead of waiting for
	// the next periodic pass. The hook is wired here rather than in the
	// constructor so a provider whose loop was never started never probes.
	p.keys.authNotify = p.probeKeyAsync
	firstCtx, cancel := context.WithTimeout(ctx, keyHealthFirstPassTimeout)
	probed, excluded := p.probeKeyHealth(firstCtx)
	cancel()
	// One line at start-up answers "is the gate live and what does it see"
	// without turning a ten-minute cadence into log noise.
	slog.Info("cline-pass key health gate started", "provider", client.ProviderClinePass,
		"interval", keyHealthProbeInterval.String(), "keys_probed", probed, "keys_excluded", excluded)
	go func() {
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

// healthScope derives the account endpoint for the configured base URL and
// discards gate verdicts recorded against a different one. Verdicts are about
// one account authority: a hot-reloaded base_url naming a mirror or a moved
// gateway makes the old verdicts meaningless, and keeping them would exclude
// healthy keys (or admit dead ones) at the new endpoint for up to one pass.
func (p *ClinePassProvider) healthScope(baseURL string) (string, error) {
	endpoint, err := quota.ClinePassUsageURL(baseURL)
	if err != nil {
		return "", err
	}
	p.healthScopeMu.Lock()
	defer p.healthScopeMu.Unlock()
	if p.healthEndpoint != "" && p.healthEndpoint != endpoint {
		p.health.Reset()
		slog.Info("cline-pass key health verdicts reset: account endpoint changed",
			"provider", client.ProviderClinePass, "previous", p.healthEndpoint, "current", endpoint)
	}
	p.healthEndpoint = endpoint
	return endpoint, nil
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
	endpoint, err := p.healthScope(cfg.ClinePass.BaseURL)
	if err != nil {
		// An unusable base_url disables the panel's fetch too; the pool's own
		// quarantine still bounds bad-key damage in the meantime.
		return 0, 0
	}
	for _, key := range keys {
		if ctx.Err() != nil {
			return probed, excluded
		}
		p.probeOneKey(ctx, endpoint, key)
		if p.health.Excluded(key) {
			excluded++
		}
		probed++
	}
	return probed, excluded
}

// probeOneKey reads one credential's subscription endpoint and records the
// verdict, logging a membership change. Callers bound ctx.
func (p *ClinePassProvider) probeOneKey(ctx context.Context, endpoint, key string) {
	prev := p.health.Excluded(key)
	_, ferr := quota.FetchClinePass(ctx, nil, endpoint, key)
	p.health.Record(key, ferr)
	if now := p.health.Excluded(key); now != prev {
		logKeyExclusion(p.Name(), key, now, ferr)
	}
}

// probeKeyAsync re-judges one key right after the upstream rejected it with an
// auth verdict, so a dead key leaves rotation in about a second instead of
// waiting for the next periodic pass. Probes for the same key are floored to
// one per minute; the probe itself runs on its own bounded context because a
// shutdown must not cancel a verdict that is already in flight.
func (p *ClinePassProvider) probeKeyAsync(key string) {
	if p.health == nil || key == "" {
		return
	}
	p.healthProbeMu.Lock()
	if p.healthProbeAt == nil {
		p.healthProbeAt = map[string]time.Time{}
	}
	if last, ok := p.healthProbeAt[key]; ok && time.Since(last) < keyHealthAsyncProbeFloor {
		p.healthProbeMu.Unlock()
		return
	}
	p.healthProbeAt[key] = time.Now()
	p.healthProbeMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), quota.RequestTimeout)
		defer cancel()
		cfg := p.atomic.Get()
		if cfg == nil {
			return
		}
		endpoint, err := p.healthScope(cfg.ClinePass.BaseURL)
		if err != nil {
			return
		}
		p.probeOneKey(ctx, endpoint, key)
	}()
}
