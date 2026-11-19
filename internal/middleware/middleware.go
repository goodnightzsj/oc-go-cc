// Package middleware provides HTTP middleware for the proxy.
package middleware

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter provides per-client IP rate limiting.
type RateLimiter struct {
	tokens    map[string]*clientTokenBucket
	mu        sync.RWMutex
	rate      int // tokens per window
	window    time.Duration
	logger    *slog.Logger
	lastSweep time.Time
}

// clientTokenBucket holds rate limit state for a single client.
type clientTokenBucket struct {
	tokens   int
	lastFill time.Time
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	if rate <= 0 {
		rate = 100 // 100 requests per window
	}
	if window == 0 {
		window = time.Minute
	}
	return &RateLimiter{
		tokens: make(map[string]*clientTokenBucket),
		rate:   rate,
		window: window,
		logger: slog.Default(),
	}
}

// Allow checks if a request from the given IP is allowed.
// Returns true if allowed, false if rate limited.
func (rl *RateLimiter) Allow(clientIP string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	bucket, exists := rl.tokens[clientIP]

	if !exists {
		rl.sweepLocked(now)
		rl.tokens[clientIP] = &clientTokenBucket{
			tokens:   rl.rate - 1,
			lastFill: now,
		}
		return true
	}

	// Refill tokens if window has passed
	elapsed := now.Sub(bucket.lastFill)
	if elapsed >= rl.window {
		bucket.tokens = rl.rate
		bucket.lastFill = now
	}

	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}

	rl.logger.Warn("rate limited", "client", clientIP, "remaining", bucket.tokens)
	return false
}

// sweepLocked drops buckets that have been idle for a full window, so a client
// that never returns cannot grow the map without bound. It runs at most once
// per window, on the first new client seen in that window.
func (rl *RateLimiter) sweepLocked(now time.Time) {
	if now.Sub(rl.lastSweep) < rl.window {
		return
	}
	rl.lastSweep = now
	for ip, bucket := range rl.tokens {
		if now.Sub(bucket.lastFill) >= 2*rl.window {
			delete(rl.tokens, ip)
		}
	}
}

// ClientIPOptions controls how the rate-limit bucket's client is identified.
type ClientIPOptions struct {
	// TrustForwardedFor makes X-Forwarded-For authoritative.
	TrustForwardedFor bool
	// TrustedProxies are the direct peers allowed to speak for a client. Empty
	// means any peer, which is only safe when the listener is reachable only
	// through the proxy.
	TrustedProxies []*net.IPNet
}

// GetClientIP extracts the client IP from an HTTP request.
//
// X-Forwarded-For is client-supplied, so it is ignored unless the operator
// declares a trusted reverse proxy. When it is trusted, the header is walked
// from the right: the first hop that is not itself a trusted proxy is the
// client. The leftmost entry is whatever the client sent, so a proxy that
// appends ($proxy_add_x_forwarded_for) would otherwise let a client choose its
// own bucket.
func GetClientIP(r *http.Request, opts ClientIPOptions) string {
	remoteHost, remoteIP := splitHost(r.RemoteAddr)
	// Join every header line: a client that sends its own X-Forwarded-For and a
	// proxy that appends one produce two lines, and Get would return only the
	// client-controlled first one.
	forwarded := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if !opts.TrustForwardedFor {
		if forwarded != "" {
			forwardedForWarning.Do(func() {
				slog.Warn("X-Forwarded-For present but trust_forwarded_for is off; rate limiting keys on the direct peer",
					"remote_addr", r.RemoteAddr)
			})
		}
		return remoteHost
	}
	// Only a trusted direct peer may speak for the client.
	if len(opts.TrustedProxies) > 0 && !ipInNets(remoteIP, opts.TrustedProxies) {
		return remoteHost
	}
	entries := make([]string, 0, 4)
	for _, raw := range strings.Split(forwarded, ",") {
		if entry := strings.TrimSpace(raw); entry != "" {
			entries = append(entries, entry)
		}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		// Entries may carry a port ("1.2.3.4:5678"); the port belongs to a
		// connection, so leaving it in would give every connection its own
		// bucket. splitHost also normalizes IPv6 spellings.
		host, ip := splitHost(entries[i])
		if ip == nil {
			// A malformed hop cannot be a trusted proxy. Falling back to the
			// direct peer is the fail-safe choice.
			return remoteHost
		}
		if !ipInNets(ip, opts.TrustedProxies) {
			return host
		}
	}
	if len(entries) > 0 {
		host, ip := splitHost(entries[0])
		if ip != nil {
			return host
		}
	}
	return remoteHost
}

// splitHost separates RemoteAddr's host from its connection port. The port is
// part of the connection, not the client: bucketing on it would give every new
// TCP connection its own bucket.
func splitHost(remoteAddr string) (string, net.IP) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return host, net.ParseIP(host)
}

func ipInNets(ip net.IP, nets []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, network := range nets {
		if network != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

// forwardedForWarning keeps the discovery hint to one line per process.
var forwardedForWarning sync.Once

// RequestIDGenerator generates unique request IDs.
type RequestIDGenerator struct {
	mu      sync.Mutex
	counter uint64
}

// NewRequestIDGenerator creates a new request ID generator.
func NewRequestIDGenerator() *RequestIDGenerator {
	return &RequestIDGenerator{}
}

// Generate creates a new unique request ID.
func (g *RequestIDGenerator) Generate() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counter++
	return fmt.Sprintf("req-%d-%d", time.Now().Unix(), g.counter)
}
