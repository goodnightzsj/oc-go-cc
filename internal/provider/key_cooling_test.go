package provider

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/core"
)

func TestKeyCoolingReportsEarliestRecovery(t *testing.T) {
	now := time.Date(2026, 11, 15, 1, 2, 3, 0, time.UTC)
	p := newKeyPool()
	p.now = func() time.Time { return now }
	keys := []string{"synthetic-a", "synthetic-b"}
	p.Reserve(keys).Settle(&client.APIError{StatusCode: http.StatusTooManyRequests, RetryAfter: "90"})
	p.Reserve(keys).Settle(&client.APIError{StatusCode: http.StatusTooManyRequests, RetryAfter: "42"})
	err := p.Reserve(keys).Err()
	var cooling *core.KeysCoolingError
	if !errors.Is(err, ErrAllKeysCooling) || !errors.As(err, &cooling) {
		t.Fatalf("missing cooling error: %v", err)
	}
	if !cooling.RetryAt.Equal(now.Add(42 * time.Second)) {
		t.Fatalf("retry at %v", cooling.RetryAt)
	}
	now = cooling.RetryAt
	sel := p.Reserve(keys)
	defer sel.Release()
	if sel.Err() != nil || sel.Key() != keys[1] {
		t.Fatalf("first recovered key unavailable: %v", sel.Err())
	}
}

func TestSingle429OnlyCoolsRejectedKey(t *testing.T) {
	for _, tc := range []struct {
		name, header string
		wait         time.Duration
	}{
		{"default", "", 30 * time.Second},
		{"header", "7", 7 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
			p := newKeyPool()
			p.now = func() time.Time { return now }
			keys := []string{"synthetic-a", "synthetic-b", "synthetic-c"}
			p.Reserve(keys).Settle(&client.APIError{StatusCode: 429, RetryAfter: tc.header})
			if !p.blockedUntil(keys[0]).Equal(now.Add(tc.wait)) || !p.blockedUntil(keys[1]).IsZero() || !p.HasUsable(keys) {
				t.Fatal("one rejection must not cool the whole pool")
			}
			next := p.Reserve(keys)
			if next.Key() != keys[1] || next.Err() != nil {
				t.Fatalf("healthy key unavailable: key=%q err=%v", next.Key(), next.Err())
			}
			next.Release()
			now = now.Add(tc.wait)
			recovered := p.Reserve(keys)
			defer recovered.Release()
			if recovered.Key() != keys[0] || recovered.Err() != nil {
				t.Fatal("key did not recover at the advertised boundary")
			}
		})
	}
}
