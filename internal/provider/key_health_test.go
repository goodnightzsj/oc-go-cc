package provider

import (
	"errors"
	"fmt"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/quota"
)

func TestKeyHealthGate_VerdictClassification(t *testing.T) {
	g := newKeyHealthGate()
	key := "synthetic-credential"

	// A successful probe admits the key.
	g.Record(key, nil)
	if g.Excluded(key) {
		t.Fatal("a successful probe must admit the key")
	}

	// Rejected statuses mark it dead: the account API said the credential is
	// invalid, unpaid, forbidden, or carries no subscription.
	for _, status := range []int{401, 402, 403, 404} {
		g.Record(key, &quota.HTTPStatusError{Status: status, Err: fmt.Errorf("HTTP %d: rejected", status)})
		if !g.Excluded(key) {
			t.Fatalf("status %d must exclude the key", status)
		}
	}

	// Unknown outcomes preserve the previous verdict in both directions, the
	// same way a single 502 does not cool a working key.
	g.Record(key, errors.New("dial tcp: i/o timeout"))
	if !g.Excluded(key) {
		t.Fatal("a network error must not lift an exclusion")
	}
	g.Record(key, &quota.HTTPStatusError{Status: 429, Err: errors.New("HTTP 429: Too Many Requests")})
	if !g.Excluded(key) {
		t.Fatal("a rate limit on the account endpoint must not lift an exclusion")
	}
	g.Record(key, &quota.HTTPStatusError{Status: 503, Err: errors.New("HTTP 503: Service Unavailable")})
	if !g.Excluded(key) {
		t.Fatal("an upstream outage must not lift an exclusion")
	}

	// A successful probe lifts it again - exclusion is a state, not a sentence.
	g.Record(key, nil)
	if g.Excluded(key) {
		t.Fatal("a later successful probe must return the key to rotation")
	}
}

func TestKeyHealthGate_FilterShapes(t *testing.T) {
	g := newKeyHealthGate()
	live, dead := "live-credential", "dead-credential"
	g.Record(dead, &quota.HTTPStatusError{Status: 404, Err: errors.New("HTTP 404: no plan history found for user")})

	keys := []string{live, dead}
	filtered := g.Filter(keys)
	if len(filtered) != 1 || filtered[0] != live {
		t.Fatalf("filter must drop only the probe-dead key, got %v", filtered)
	}

	// A pool that would lose every key stays unfiltered: every account probe
	// rejecting is indistinguishable from a moved endpoint.
	g.Record(live, &quota.HTTPStatusError{Status: 404, Err: errors.New("HTTP 404: no plan history found for user")})
	if again := g.Filter(keys); len(again) != 2 {
		t.Fatalf("fail-open: a filter that would empty the pool must return the input, got %v", again)
	}

	// A lone key is never filtered, mirroring the scheduler's own rule.
	if lone := g.Filter([]string{dead}); len(lone) != 1 || lone[0] != dead {
		t.Fatalf("a lone key must pass through unfiltered, got %v", lone)
	}

	// A nil gate is pass-through.
	var nilGate *keyHealthGate
	if passthrough := nilGate.Filter(keys); len(passthrough) != 2 {
		t.Fatalf("a nil gate must not filter, got %v", passthrough)
	}
}

// The scheduler stamps the credential label on the API error the attempt
// produced, so a failure record can name the rejected key.
func TestSelectionSettleStampsKeyHintOnAPIError(t *testing.T) {
	p := newKeyPool()
	sel := p.Reserve([]string{"synthetic-credential-9f2c"})
	apiErr := &client.APIError{StatusCode: 403, Body: "not subscribed"}
	sel.Settle(apiErr)
	if want := "****9f2c"; apiErr.KeyHint != want {
		t.Fatalf("Settle stamped %q, want %q", apiErr.KeyHint, want)
	}

	// A hint somebody recorded earlier is not overwritten.
	sel = p.Reserve([]string{"synthetic-credential-9f2c"})
	labeled := &client.APIError{StatusCode: 429, KeyHint: "****prev"}
	sel.Settle(labeled)
	if labeled.KeyHint != "****prev" {
		t.Fatalf("Settle overwrote an existing hint: %q", labeled.KeyHint)
	}

	// A dialect error without a key label stays untouched.
	sel = p.Reserve([]string{"synthetic-credential-9f2c"})
	plain := errors.New("connection reset")
	sel.Settle(plain)
	var extracted *client.APIError
	if errors.As(plain, &extracted) {
		t.Fatal("plain error must not become an API error")
	}
}
