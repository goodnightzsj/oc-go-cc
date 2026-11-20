// Package provider implements the core.Provider interface for all supported
// upstream LLM providers.
package provider

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/metrics"
)

// baseProvider holds shared HTTP transport and key scheduling used by all
// provider implementations in this package.
type baseProvider struct {
	atomic     *config.AtomicConfig
	httpClient *http.Client
	keys       *keyPool
	// health holds subscription-probe verdicts; nil on providers without a
	// probe. Filter is nil-safe, so reserveKey need not branch.
	health *keyHealthGate
}

func differingResponseModel(reported, requested string) string {
	if strings.TrimSpace(reported) == "" || strings.EqualFold(strings.TrimSpace(reported), strings.TrimSpace(requested)) {
		return ""
	}
	return reported
}

// newBaseProvider creates a baseProvider with a shared HTTP transport tuned
// for high-concurrency upstream calls.
func newBaseProvider(atomic *config.AtomicConfig) baseProvider {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		MaxConnsPerHost:     50,
		DisableKeepAlives:   false,
		Proxy:               http.ProxyFromEnvironment,
	}
	return baseProvider{
		atomic: atomic,
		httpClient: &http.Client{
			Transport: &countingTransport{transport},
		},
		keys: newKeyPool(),
	}
}

// keyHintBody labels a response body with the credential that carried it.
//
// The history record needs to say which account served a request. The handler
// never sees the selection, so the label rides the body through the same
// observation chain the channel and model already use. Wrapping here rather
// than in each provider keeps one implementation: the alternative is six
// copies that each have to remember to forward it.
type keyHintBody struct {
	io.ReadCloser
	hint string
}

func (b *keyHintBody) KeyHint() string { return b.hint }

// withKeyHint attaches sel's label to body. A nil body passes through so a
// caller can wrap without checking.
func (b *baseProvider) withKeyHint(body io.ReadCloser, sel *selection) io.ReadCloser {
	if body == nil {
		return nil
	}
	return &keyHintBody{ReadCloser: body, hint: sel.KeyHint()}
}

// do observes each upstream response before any provider consumes its body.
func (b *baseProvider) do(req *http.Request) (*http.Response, error) {
	logRequestControls(req, slog.Default())
	resp, err := b.httpClient.Do(req)
	if err == nil {
		metrics.ObserveGenerationResponse(req.Context(), resp)
	}
	return resp, err
}

// doSelected settles HTTP rejections immediately; 2xx transfers health ownership
// to Execute's parser or the source stream body. Local failures stay neutral.
// Providers needing an error body for classification use do before transferring.
func (b *baseProvider) doSelected(req *http.Request, sel *selection) (*http.Response, error) {
	resp, err := b.do(req)
	outcome := err
	if err == nil && resp.StatusCode >= http.StatusBadRequest {
		outcome = &client.APIError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		ownSelectedResponse(req.Context(), resp, sel)
	} else {
		sel.Settle(outcome)
	}
	return resp, err
}

// Count at the shared transport boundary, including retries and redirects, but
// not candidates rejected before sending. Embedded transport keeps idle cleanup.
type countingTransport struct{ *http.Transport }

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	metrics.RecordUpstreamCall(req.Context())
	return t.Transport.RoundTrip(req)
}

// reserveKey picks the key for one request and returns a handle the caller must
// settle with the outcome.
//
// This replaced a round-robin counter that moved every other request onto the
// next key regardless of how the previous one was doing. Selection is now
// fill-first and health-aware: a working key keeps taking traffic, and a key
// steps aside only after the upstream has actually rejected it. The returned
// handle carries both the credential and the health record, so the caller only
// has to pass the send result back - see selection.Settle.
//
// Callers defer Release immediately; doSelected hands successful responses to
// their consumer without releasing the in-flight reservation at HTTP headers.
func (b *baseProvider) reserveKey(keys []string) *selection {
	// Probe-dead credentials never reach the scheduler; the gate is nil for
	// providers without a subscription probe and fail-open by construction.
	keys = b.health.Filter(keys)
	if b.keys == nil {
		// A zero-value baseProvider would otherwise panic; treat it as a pool
		// with no keys so the caller reports "no API key configured".
		return &selection{}
	}
	return b.keys.Reserve(keys)
}

// reserveKeyChecked is reserveKey plus the two ways a reservation can fail
// before anything is sent: no key configured, and every key resting after a
// rejection. Callers that need to report the difference use this; the rest can
// keep checking Key() == "" for the unconfigured case alone.
//
// The resting case must not be sent: those keys are resting because the upstream
// rejected them, so the attempt is known to be doomed and a retrying caller
// would spend one doomed request per retry.
func (b *baseProvider) reserveKeyChecked(provider string, keys []string) (*selection, error) {
	sel := b.reserveKey(keys)
	if err := sel.Err(); err != nil {
		return sel, fmt.Errorf("%s: %w", provider, err)
	}
	if sel.Key() == "" {
		return sel, fmt.Errorf("no API key configured for %s", provider)
	}
	return sel, nil
}

// pools maps a provider name to its scheduler, so the fallback handler can ask
// "can this platform still serve?" without holding a reference to every
// provider instance. Registration happens where the provider is constructed.
var (
	poolsMu sync.RWMutex
	pools   = map[string]*keyPool{}
)

// registerPool records a provider's scheduler for availability queries.
func registerPool(provider string, pool *keyPool) {
	poolsMu.Lock()
	defer poolsMu.Unlock()
	pools[provider] = pool
}

// poolFor returns a provider's scheduler, or nil when it never registered one.
func poolFor(provider string) *keyPool {
	poolsMu.RLock()
	defer poolsMu.RUnlock()
	return pools[provider]
}

// gates maps a provider name to its subscription-probe gate, when it has one.
// Registered beside the pool for the same reason: the fallback handler asks
// about a platform by name and must see the same key set Reserve would.
var (
	gatesMu sync.RWMutex
	gates   = map[string]*keyHealthGate{}
)

func registerGate(provider string, gate *keyHealthGate) {
	gatesMu.Lock()
	defer gatesMu.Unlock()
	gates[provider] = gate
}

// gateFor returns a provider's gate, or nil when it has no probe. A nil gate
// passes keys through unchanged, so callers need no branch.
func gateFor(provider string) *keyHealthGate {
	gatesMu.RLock()
	defer gatesMu.RUnlock()
	return gates[provider]
}

// HasUsableKey reports whether any key in this provider's pool can take a
// request right now.
//
// The fallback handler uses it to tell "this credential is bad, the platform's
// next model may work" from "this platform has nothing left to try". Counting
// keys answers the first question only by accident: three configured keys can
// all be resting.
//
// A pool with no keys reports false, and so does a provider whose scheduler was
// never initialised - both mean there is nothing to send with.
func (b *baseProvider) HasUsableKey(keys []string) bool {
	if b.keys == nil {
		return false
	}
	// A pool whose every key the probe rejected has nothing to offer even
	// though Filter, being fail-open, still returns those keys. Reporting
	// "usable" here is what made a dead single-key platform spend one doomed
	// request per fallback model.
	if b.health.ExcludesAll(keys) {
		return false
	}
	// Gate first, so "usable" means the same set of keys Reserve would pick
	// from. A pool whose remaining keys are all cooling is not usable just
	// because the probe-dead keys are idle.
	keys = b.health.Filter(keys)
	// HasUsableForFallback, not HasUsable: a lone key that just earned an auth
	// quarantine is still allowed one attempt by Reserve, but the fallback
	// handler must not spend the platform's remaining models on it.
	return b.keys.HasUsableForFallback(keys)
}

// HasUsable reports whether any of the given keys is not resting.
func (p *keyPool) HasUsable(keys []string) bool {
	// A lone key is never withheld, so it is always usable.
	if len(keys) <= 1 {
		return len(keys) == 1
	}
	return p.HasUsableForFallback(keys)
}

// HasUsableForFallback answers the fallback handler's question - is another
// attempt on this platform worth making - without the lone-key exemption.
// Reserve still lets a single key through once, but after an auth rejection
// quarantines it every remaining model would spend the same doomed request.
func (p *keyPool) HasUsableForFallback(keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	for _, key := range keys {
		if st := p.states[key]; st == nil || !st.blockedUntil.After(now) {
			return true
		}
	}
	return false
}

// KeyAvailability answers the fallback handler's question about a named
// provider, reading each provider's key list from the live config.
type KeyAvailability struct {
	atomic *config.AtomicConfig
}

// NewKeyAvailability builds the availability source over the live config, which
// is where each provider's key list comes from.
func NewKeyAvailability(atomic *config.AtomicConfig) *KeyAvailability {
	return &KeyAvailability{atomic: atomic}
}

// CredentialsExhausted reports a durable reason this provider cannot serve:
// it has no key at all, or the subscription probe has excluded every key it
// has. A key that is merely resting is deliberately ignored - a 30-second
// cooldown would read as an outage, and the health endpoint that reports this
// is what the deploy gate decides to roll back on.
func (k *KeyAvailability) CredentialsExhausted(provider string) bool {
	if k == nil || k.atomic == nil {
		return false
	}
	cfg := k.atomic.Get()
	if cfg == nil {
		return false
	}
	keys := cfg.ProviderAPIKeys(provider)
	if len(keys) == 0 {
		return true
	}
	return gateFor(provider).ExcludesAll(keys)
}

// HasUsableKey reports whether the named provider has a key that can take a
// request. A provider with no scheduler registered reports false.
func (k *KeyAvailability) HasUsableKey(provider string) bool {
	if k == nil || k.atomic == nil {
		return false
	}
	cfg := k.atomic.Get()
	if cfg == nil {
		return false
	}
	pool := poolFor(provider)
	if pool == nil {
		return false
	}
	// The gate is consulted so "cannot serve" agrees with what Reserve would
	// actually pick: probe-dead keys are not candidates, and keys they crowded
	// out are back in the usable set. A pool the probe rejected entirely is
	// unusable even when Filter's fail-open path returns it unchanged.
	keys := cfg.ProviderAPIKeys(provider)
	gate := gateFor(provider)
	if gate.ExcludesAll(keys) {
		return false
	}
	return pool.HasUsableForFallback(gate.Filter(keys))
}
