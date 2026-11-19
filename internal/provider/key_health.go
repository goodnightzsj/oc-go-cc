package provider

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/routatic/proxy/internal/quota"
)

// keyHealthGate keeps one credential's subscription-probe verdict and removes
// probe-dead keys from rotation before the scheduler sees them.
//
// Why a gate and not a longer quarantine: the pool's auth quarantine reacts
// only after a doomed request has been spent, and it expires - a key whose
// account has no subscription re-enters rotation every five minutes and burns
// another doomed request whenever the healthy keys are busy. The subscription
// probe (the same account endpoint the Quota tab reads) knows the account is
// dead without spending an inference request, so the gate holds it out of
// rotation until a probe succeeds again.
//
// The gate is fail-open on purpose. It never filters a lone key, mirroring the
// scheduler's own lone-key rule, a verdict it cannot classify leaves the
// previous state untouched, and a filter that would exclude every key returns
// the input unchanged: an upstream that rejects every account probe is
// indistinguishable from a moved endpoint, and guessing wrong takes the whole
// platform dark. The pool's own auth quarantine still bounds the damage in
// that case.
//
// Health is per credential, not per credential x model. An account that can
// reach its own plan endpoint but is not entitled to one model still fails
// that model's requests; that case degrades to the pool's auth quarantine and
// the fallback chain, because probing every model on every key would spend
// what it measures.
type keyHealthGate struct {
	mu    sync.Mutex
	state map[string]healthVerdict
}

type healthVerdict struct {
	unhealthy bool
	reason    string
}

func newKeyHealthGate() *keyHealthGate {
	return &keyHealthGate{state: map[string]healthVerdict{}}
}

// Filter drops the keys the probe has marked dead. A nil gate and a pool of
// one are passed through unchanged.
func (g *keyHealthGate) Filter(keys []string) []string {
	if g == nil || len(keys) <= 1 {
		return keys
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if v, ok := g.state[key]; ok && v.unhealthy {
			continue
		}
		out = append(out, key)
	}
	if len(out) == 0 {
		return keys
	}
	return out
}

// Excluded reports whether the probe currently holds this key out of rotation.
func (g *keyHealthGate) Excluded(key string) bool {
	if g == nil || key == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	v, ok := g.state[key]
	return ok && v.unhealthy
}

// ExcludesAll reports whether the probe holds every one of keys out of
// rotation. It is the question Filter cannot answer: Filter is fail-open and
// returns its input when nothing is left, which is right for scheduling (one
// attempt beats none) but wrong for deciding whether a failed platform's
// remaining models are worth trying. A single-key platform is exactly this
// case - without this check the probe's verdict never affects anything.
func (g *keyHealthGate) ExcludesAll(keys []string) bool {
	if g == nil || len(keys) == 0 {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, key := range keys {
		v, ok := g.state[key]
		if !ok || !v.unhealthy {
			return false
		}
	}
	return true
}

// Record stores one probe outcome. A nil error means the account answered, so
// any exclusion is lifted; a rejected status (401/402/403/404 - invalid,
// unpaid, forbidden, or no subscription) marks the key dead; every other
// outcome (429, 5xx, network) is "unknown" and preserves what was known, the
// same way a single 502 does not cool a working key.
func (g *keyHealthGate) Record(key string, err error) {
	if g == nil || key == "" {
		return
	}
	unhealthy, reason := false, ""
	if err != nil {
		status, ok := probeStatusOf(err)
		if !ok {
			return
		}
		switch status {
		case 401, 402, 403, 404:
			unhealthy, reason = true, err.Error()
		default:
			return
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.state[key] = healthVerdict{unhealthy: unhealthy, reason: reason}
}

// Reset discards every verdict. Verdicts are about one account authority, so a
// different endpoint makes the old ones meaningless rather than merely stale.
func (g *keyHealthGate) Reset() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.state = map[string]healthVerdict{}
}

// probeStatusOf extracts the HTTP status from a quota fetch error. Errors that
// carry no status (network, timeouts) report unknown.
func probeStatusOf(err error) (int, bool) {
	var statusErr *quota.HTTPStatusError
	if errors.As(err, &statusErr) && statusErr != nil {
		return statusErr.Status, true
	}
	return 0, false
}

// logKeyExclusion reports a rotation-membership change. The hint is the same
// masked label the history column uses, so no line carries a credential.
func logKeyExclusion(provider, key string, excluded bool, err error) {
	hint := KeyMaskHint(key)
	switch {
	case excluded:
		detail := ""
		if err != nil {
			detail = err.Error()
		}
		slog.Warn("key held out of rotation: subscription probe rejected it", "provider", provider, "key_hint", hint, "error", detail)
	default:
		slog.Info("key returned to rotation: subscription probe succeeded", "provider", provider, "key_hint", hint)
	}
}
