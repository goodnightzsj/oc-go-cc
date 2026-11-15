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
