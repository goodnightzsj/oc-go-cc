// Package provider implements the core.Provider interface for all supported
// upstream LLM providers.
package provider

import (
	"fmt"
	"net/http"
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

// doSelected records health at the shared HTTP boundary. Encoding and request
// construction failures never reach here; the caller's deferred Release leaves
// those neutral. Bodies remain owned by the protocol-specific caller.
func (b *baseProvider) doSelected(req *http.Request, sel *selection) (*http.Response, error) {
	resp, err := b.httpClient.Do(req)
	outcome := err
	if err == nil && resp.StatusCode >= http.StatusBadRequest {
		outcome = &client.APIError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}
	sel.Settle(outcome)
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
// Callers defer Release immediately after reservation and send through
// doSelected, so local failures and HTTP results have one lifecycle owner.
func (b *baseProvider) reserveKey(keys []string) *selection {
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
	if b.keys == nil || len(keys) == 0 {
		return false
	}
	return b.keys.HasUsable(keys)
}

// HasUsable reports whether any of the given keys is not resting.
func (p *keyPool) HasUsable(keys []string) bool {
	// A lone key is never withheld, so it is always usable.
	if len(keys) <= 1 {
		return len(keys) == 1
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
	return pool.HasUsable(cfg.ProviderAPIKeys(provider))
}
