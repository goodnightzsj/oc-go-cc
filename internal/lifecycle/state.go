package lifecycle

import "sync/atomic"

// State tracks request activity and whether the server is draining for shutdown.
type State struct {
	draining atomic.Bool
	active   atomic.Int64
}

// NewState creates a new lifecycle state tracker.
func NewState() *State {
	return &State{}
}

// BeginRequest marks a request as active and returns a cleanup function.
func (s *State) BeginRequest() func() {
	s.active.Add(1)
	return func() {
		s.active.Add(-1)
	}
}

// ActiveRequests returns the current number of active requests.
func (s *State) ActiveRequests() int64 {
	return s.active.Load()
}

// BeginDrain marks the server as draining.
func (s *State) BeginDrain() {
	s.draining.Store(true)
}

// IsDraining reports whether the server is draining.
func (s *State) IsDraining() bool {
	return s.draining.Load()
}
