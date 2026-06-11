package lifecycle

import "testing"

func TestStateTracksActiveRequestsAndDrain(t *testing.T) {
	state := NewState()
	if state.IsDraining() {
		t.Fatal("new state should not be draining")
	}
	if got := state.ActiveRequests(); got != 0 {
		t.Fatalf("ActiveRequests() = %d, want 0", got)
	}

	doneA := state.BeginRequest()
	doneB := state.BeginRequest()
	if got := state.ActiveRequests(); got != 2 {
		t.Fatalf("ActiveRequests() = %d, want 2", got)
	}

	state.BeginDrain()
	if !state.IsDraining() {
		t.Fatal("state should be draining after BeginDrain")
	}

	doneA()
	doneB()
	if got := state.ActiveRequests(); got != 0 {
		t.Fatalf("ActiveRequests() = %d, want 0", got)
	}
}
