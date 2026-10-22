package provider

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/client"
)

// Key scheduling defaults. They mirror the model-level circuit breaker's shape
// (three failures, thirty seconds) so an operator reading one set of numbers can
// reason about the other, and they match what comparable proxies converge on.
const (
	// transientThreshold is how many consecutive transient failures (5xx, dial
	// errors) a key may accumulate before it is cooled. A single blip is not
	// enough: upstream gateways routinely return one 502 under load and serve
	// the next request fine, and cooling on the first would move traffic off a
	// key that is working.
	transientThreshold = 3

	// transientCooldown is how long a key rests after hitting the threshold.
	transientCooldown = 30 * time.Second

	// rateLimitCooldown is the fallback rest for a 429 that carried no usable
	// Retry-After. The header wins when present.
	rateLimitCooldown = 30 * time.Second

	// authQuarantine is how long a key rests after the upstream rejects the
	// credential itself (401/402/403). These are not transient: retrying
	// immediately spends a doomed request per attempt, so the key is set aside
	// at once rather than after accumulating failures. It is never permanent -
	// the timer expiring, or any later success, returns the key to service.
	authQuarantine = 5 * time.Minute
)

// keyState is one key's health. The zero value is healthy.
type keyState struct {
	// consecutive counts transient failures since the last success. Reset by a
	// success and by the cooldown expiring.
	consecutive int
	// blockedUntil is when the key may take traffic again. Zero means now.
	blockedUntil time.Time
	// inFlight is how many requests currently hold this key. It keeps a burst
	// of concurrent requests from all picking the same key before any of them
	// has reported a result.
	inFlight int
}

// keyPool schedules requests across one platform's API keys.
//
// It exists because plain round-robin moved traffic off a healthy key: with two
// keys configured, every other request went to the second one no matter how well
// the first was working, so a perfectly good key's traffic was split for no
// reason. Selection here is fill-first - the earliest configured key that is not
// resting takes the request - and a key only steps aside once the upstream has
// actually said something is wrong with it.
//
// A resting key is skipped, not disabled. A multi-key pool with every key
// resting rejects the reservation until a key recovers.
type keyPool struct {
	mu     sync.Mutex
	states map[string]*keyState
	// now is the clock, injectable so tests can advance time without sleeping.
	now func() time.Time
}

func newKeyPool() *keyPool {
	return &keyPool{states: map[string]*keyState{}, now: time.Now}
}

// ErrAllKeysCooling reports that every configured key is resting after an
// upstream rejection, so there is nothing to send with. It is distinct from an
// empty pool, which means no key was configured at all.
var ErrAllKeysCooling = errors.New("every API key is resting after an upstream rejection")

// selection is one reserved key. Defer Release at its owner; doSelected settles
// HTTP outcomes. Either path releases in-flight exactly once.
type selection struct {
	pool *keyPool
	key  string
	// err is set when no key could be reserved, so the caller can report why
	// instead of sending with an empty credential.
	err error
	// done guards against a double Settle, which would decrement in-flight
	// twice and could unblock a key that is still in use.
	done bool
}

// KeyHint labels the reserved credential for the history record: the last four
// characters behind a mask, so the column can say which account served a
// request without the database ever holding a credential. Empty when nothing
// was reserved.
func (s *selection) KeyHint() string {
	if s == nil || s.key == "" {
		return ""
	}
	if len(s.key) <= 4 {
		return "****"
	}
	return "****" + s.key[len(s.key)-4:]
}

// Key is the credential to send. Empty when no key could be reserved.
func (s *selection) Key() string {
	if s == nil {
		return ""
	}
	return s.key
}

// Err reports why no key was reserved, or nil when one was.
func (s *selection) Err() error {
	if s == nil {
		return nil
	}
	return s.err
}

// Reserve picks the key to use for one request.
//
// Fill-first: the first key in configured order that is not resting and is not
// already saturated by in-flight requests wins.
//
// A pool of one key never rests. Resting a key means stepping aside for another
// one, so with nothing to step aside to it would only stop the platform serving
// entirely - a 401 on the sole key would take the platform down for the whole
// quarantine window instead of letting the caller's own fallback handle it. The
// single key is still reported to Settle, so its health is available to whoever
// asks; it is just never withheld from service.
//
// When every key of a multi-key pool is resting the reservation comes back
// empty and ErrAllKeysCooling explains why. Sending anyway would be worse than
// failing: those keys are resting because the upstream rejected them, so the
// attempt is known to be doomed, and a caller that retries on failure would burn
// one doomed request per retry - the amplification this replaced round-robin to
// avoid.
func (p *keyPool) Reserve(keys []string) *selection {
	if len(keys) == 0 {
		return &selection{pool: p}
	}
	if len(keys) == 1 {
		// See the doc comment: a lone key is never withheld.
		p.mu.Lock()
		defer p.mu.Unlock()
		st := p.stateFor(keys[0])
		st.inFlight++
		return &selection{pool: p, key: keys[0]}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()

	best := ""
	bestScore := time.Duration(1<<62 - 1)
	allResting := true
	for _, key := range keys {
		st := p.stateFor(key)
		wait := st.blockedUntil.Sub(now)
		if wait <= 0 {
			wait = 0
			allResting = false
		}
		// A key that is ready but busy is scored by how many requests already
		// hold it, so a burst spreads across the pool rather than stacking on
		// the first key. This is the only place in-flight matters: once a key
		// has reported a failure its blockedUntil dominates the ordering.
		score := wait*time.Duration(len(keys)+1) + time.Duration(st.inFlight)
		if score < bestScore {
			bestScore = score
			best = key
		}
	}
	if allResting {
		return &selection{pool: p, err: ErrAllKeysCooling}
	}
	st := p.stateFor(best)
	st.inFlight++
	return &selection{pool: p, key: best}
}

// Settle reports what happened to a reserved key.
//
// A nil error means the request reached the upstream and it answered 2xx, which
// clears the key's state outright: a success is direct proof the key works, so
// any cooldown it was serving is over. An error is classified by status - see
// classify - and only a verdict about the key changes its state. A 400 from a
// malformed request or a client cancellation says nothing about the key and is
// not cooled. Cancellation is neutral and preserves concurrent health verdicts;
// a non-key-related HTTP rejection retains the existing healthy verdict.
func (s *selection) Settle(err error) {
	s.settle(classify(err), err)
}

// Release is deferred immediately after reserving. Local failures are neutral;
// a completed HTTP verdict wins, so releasing again cannot erase its health.
func (s *selection) Release() {
	s.settle(verdictNeutral, nil)
}

func (s *selection) settle(verdict verdict, err error) {
	if s == nil || s.pool == nil || s.key == "" {
		return
	}
	p := s.pool
	p.mu.Lock()
	defer p.mu.Unlock()
	if s.done {
		return
	}
	s.done = true
	st := p.stateFor(s.key)
	if st.inFlight > 0 {
		st.inFlight--
	}
	now := p.now()

	switch verdict {
	case verdictHealthy:
		st.consecutive = 0
		st.blockedUntil = time.Time{}
	case verdictAuth:
		// Rejected credential or exhausted budget: no number of retries fixes
		// it, so set it aside now rather than after the transient threshold.
		st.consecutive = 0
		st.blockedUntil = now.Add(authQuarantine)
	case verdictRateLimited:
		st.consecutive = 0
		st.blockedUntil = now.Add(retryAfter(err, rateLimitCooldown))
	case verdictTransient:
		st.consecutive++
		if st.consecutive >= transientThreshold {
			st.blockedUntil = now.Add(transientCooldown)
			// Reset so that when the cooldown expires the key starts fresh: the
			// failures that opened it have been paid for by the rest.
			st.consecutive = 0
		}
	}
}

// stateFor returns the key's state, creating it on first use. Callers hold mu.
func (p *keyPool) stateFor(key string) *keyState {
	st := p.states[key]
	if st == nil {
		st = &keyState{}
		p.states[key] = st
	}
	return st
}

// blockedUntil reports when a key may serve again, for tests and diagnostics.
func (p *keyPool) blockedUntil(key string) time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	if st := p.states[key]; st != nil {
		return st.blockedUntil
	}
	return time.Time{}
}

// verdict is what a failed request says about the key that carried it.
type verdict int

const (
	// verdictHealthy is a success, or a failure that is not the key's fault.
	verdictHealthy verdict = iota
	// verdictAuth is a rejected credential or an exhausted budget.
	verdictAuth
	// verdictRateLimited is a 429.
	verdictRateLimited
	// verdictTransient is a 5xx, a dial error, or a stream that broke midway.
	verdictTransient
	// verdictNeutral releases a reservation without changing another request's verdict.
	verdictNeutral
)

// classify maps an error to what it says about the key.
//
// Anything that is not a status the key is responsible for returns
// verdictHealthy. Cancellation returns verdictNeutral; neither cools the key.
func classify(err error) verdict {
	if err == nil {
		return verdictHealthy
	}
	if errors.Is(err, context.Canceled) {
		return verdictNeutral
	}
	status, ok := statusOf(err)
	if !ok {
		// No HTTP status: a dial failure, a reset, a timeout. Those are the
		// key's problem only in the sense that they are the connection's, and
		// they are exactly what the transient threshold exists to absorb.
		return verdictTransient
	}
	switch {
	case status == http.StatusUnauthorized,
		status == http.StatusPaymentRequired,
		status == http.StatusForbidden:
		return verdictAuth
	case status == http.StatusTooManyRequests:
		return verdictRateLimited
	case status == http.StatusRequestTimeout, status >= 500:
		return verdictTransient
	default:
		// Other 4xx: the request was wrong, the key was not.
		return verdictHealthy
	}
}

// statusOf extracts an HTTP status from an error chain.
func statusOf(err error) (int, bool) {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr != nil {
		return apiErr.StatusCode, true
	}
	return 0, false
}

// retryAfter reads a Retry-After duration the error carries, falling back to
// def. The header is parsed as either delta-seconds or an HTTP date; an
// unparseable or absent value yields the default rather than zero, because
// retrying a rate-limited key immediately is what the cooldown exists to stop.
func retryAfter(err error, def time.Duration) time.Duration {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) || apiErr == nil {
		return def
	}
	raw := strings.TrimSpace(apiErr.RetryAfter)
	if raw == "" {
		return def
	}
	if secs, convErr := strconv.Atoi(raw); convErr == nil {
		if secs <= 0 {
			return def
		}
		return time.Duration(secs) * time.Second
	}
	if at, parseErr := http.ParseTime(raw); parseErr == nil {
		if d := time.Until(at); d > 0 {
			return d
		}
		return def
	}
	return def
}
