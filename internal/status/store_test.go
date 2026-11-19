package status

import (
	"testing"
	"time"
)

func TestPublishFeedsTheSnapshot(t *testing.T) {
	store := NewStore(time.Minute)
	if snap := store.Snapshot(); !snap.Stale || snap.Source != "empty" {
		t.Fatalf("an untouched store must report empty/stale, got %+v", snap)
	}

	store.Publish(Snapshot{
		Routing: RoutingSnapshot{Scenario: "default", ModelID: "glm-5.2"},
		Context: ContextSnapshot{InputTokens: 100, MaxTokens: 1000, Percent: 10},
	})

	snap := store.Snapshot()
	if snap.Stale || snap.Source != "proxy" {
		t.Fatalf("a published snapshot must be fresh, got %+v", snap)
	}
	if snap.Routing.ModelID != "glm-5.2" || snap.Context.Percent != 10 {
		t.Fatalf("published fields were not retained: %+v", snap)
	}
	if snap.Proxy.PID == 0 {
		t.Fatal("the proxy identity must be filled in")
	}
}
