package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mustNets(t *testing.T, cidrs ...string) []*net.IPNet {
	t.Helper()
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatalf("parse %s: %v", cidr, err)
		}
		out = append(out, network)
	}
	return out
}

func TestGetClientIPIgnoresUntrustedForwardedFor(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.9:54321"
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")

	// The header is client-supplied; without a declared trusted proxy it must
	// not decide the rate-limit bucket.
	if got := GetClientIP(r, ClientIPOptions{}); got != "192.0.2.9" {
		t.Fatalf("untrusted GetClientIP = %q, want the RemoteAddr host", got)
	}
	// With a trusted proxy, the rightmost entry is what that proxy saw. The
	// leftmost is client-supplied and must not be trusted.
	if got := GetClientIP(r, ClientIPOptions{TrustForwardedFor: true}); got != "10.0.0.1" {
		t.Fatalf("trusted GetClientIP = %q, want the rightmost forwarded entry", got)
	}
	r.Header.Set("X-Forwarded-For", "spoofed-value, 198.51.100.4")
	if got := GetClientIP(r, ClientIPOptions{TrustForwardedFor: true}); got != "198.51.100.4" {
		t.Fatalf("trusted GetClientIP = %q, a client-prepended value must not win", got)
	}
}

func TestGetClientIPRequiresATrustedDirectPeer(t *testing.T) {
	trusted := mustNets(t, "10.0.0.0/8", "127.0.0.1/32")

	fromProxy := httptest.NewRequest("GET", "/", nil)
	fromProxy.RemoteAddr = "10.1.2.3:5555"
	fromProxy.Header.Set("X-Forwarded-For", "198.51.100.7, 10.9.9.9")
	if got := GetClientIP(fromProxy, ClientIPOptions{TrustForwardedFor: true, TrustedProxies: trusted}); got != "198.51.100.7" {
		t.Fatalf("GetClientIP = %q, want the first untrusted hop", got)
	}

	// A direct client outside the trusted range cannot speak for anyone.
	direct := httptest.NewRequest("GET", "/", nil)
	direct.RemoteAddr = "192.0.2.9:5555"
	direct.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := GetClientIP(direct, ClientIPOptions{TrustForwardedFor: true, TrustedProxies: trusted}); got != "192.0.2.9" {
		t.Fatalf("GetClientIP = %q, an untrusted peer's header must be ignored", got)
	}
}

func TestGetClientIPJoinsHeaderLinesAndStripsPorts(t *testing.T) {
	trusted := mustNets(t, "127.0.0.1/32")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	// A client-supplied line followed by the proxy's own line.
	r.Header.Add("X-Forwarded-For", "spoofed-value")
	r.Header.Add("X-Forwarded-For", "198.51.100.7:44321")
	if got := GetClientIP(r, ClientIPOptions{TrustForwardedFor: true, TrustedProxies: trusted}); got != "198.51.100.7" {
		t.Fatalf("GetClientIP = %q, want the rightmost hop without its port", got)
	}

	// An unparseable hop falls back to the direct peer rather than inventing a
	// bucket key.
	r.Header = http.Header{}
	r.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := GetClientIP(r, ClientIPOptions{TrustForwardedFor: true, TrustedProxies: trusted}); got != "127.0.0.1" {
		t.Fatalf("GetClientIP = %q, want the trusted direct peer", got)
	}
}

func TestGetClientIPStripsConnectionPort(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:60123"
	if got := GetClientIP(r, ClientIPOptions{}); got != "127.0.0.1" {
		t.Fatalf("GetClientIP = %q, want 127.0.0.1", got)
	}
}

func TestRateLimiterSweepsIdleBuckets(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute)
	rl.tokens["stale"] = &clientTokenBucket{tokens: 1, lastFill: time.Now().Add(-3 * time.Minute)}
	rl.lastSweep = time.Now().Add(-2 * time.Minute)

	if !rl.Allow("fresh") {
		t.Fatal("first request from a new client must be allowed")
	}
	if _, ok := rl.tokens["stale"]; ok {
		t.Fatal("a bucket idle for more than two windows must be evicted")
	}
	if _, ok := rl.tokens["fresh"]; !ok {
		t.Fatal("the new client's bucket must be kept")
	}
}

func TestGetClientIPAllTrustedHopsFallBackToPeer(t *testing.T) {
	trusted := mustNets(t, "10.0.0.0/8")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.1.2.3:5555"
	r.Header.Set("X-Forwarded-For", "10.9.9.9, 10.8.8.8")
	// Every hop is a trusted proxy, so the leftmost entry is still whatever the
	// client typed. Only the direct peer is trustworthy.
	if got := GetClientIP(r, ClientIPOptions{TrustForwardedFor: true, TrustedProxies: trusted}); got != "10.1.2.3" {
		t.Fatalf("GetClientIP = %q, want the direct peer", got)
	}
}

func TestGetClientIPNormalizesIPv6(t *testing.T) {
	trusted := mustNets(t, "127.0.0.1/32")
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "2001:0db8:0:0:0:0:0:1")
	if got := GetClientIP(r, ClientIPOptions{TrustForwardedFor: true, TrustedProxies: trusted}); got != "2001:db8::1" {
		t.Fatalf("GetClientIP = %q, want the canonical IPv6 form", got)
	}
}

func TestRateLimiterConfigureResetsBuckets(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	if !rl.Allow("client") {
		t.Fatal("first request must pass")
	}
	if rl.Allow("client") {
		t.Fatal("the old one-request limit must apply")
	}
	rl.Configure(2, time.Minute)
	if !rl.Allow("client") {
		t.Fatal("a new policy starts from a clean bucket")
	}
}
