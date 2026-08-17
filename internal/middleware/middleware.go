// Package middleware provides HTTP middleware for the proxy.
package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter provides per-client IP rate limiting.
type RateLimiter struct {
	tokens map[string]*clientTokenBucket
	mu     sync.RWMutex
	rate   int // tokens per window
	window time.Duration
	logger *slog.Logger
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

// GetClientIP extracts the client IP from an HTTP request.
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For first (if behind a proxy)
	// X-Forwarded-For can contain multiple IPs: "client, proxy1, proxy2"
	// We want the first (leftmost) IP which is the original client.
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first, _, _ := strings.Cut(forwarded, ",")
		return strings.TrimSpace(first)
	}
	// Fall back to RemoteAddr
	return r.RemoteAddr
}

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
