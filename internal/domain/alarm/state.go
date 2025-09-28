package alarm

import "time"

// Actor identifies who performed an action in the system.
type Actor struct {
	// Hostname is the machine name where the action was performed.
	Hostname string
	// Username is the system user who triggered the action.
	Username string
}

// Clone returns a deep copy of the actor.
func (a *Actor) Clone() *Actor {
	if a == nil {
		return nil
	}

	cloned := *a

	return &cloned
}

// State represents the alarm status at a specific point in time.
type State struct {
	// Timestamp is when the alarm state was last changed.
	Timestamp time.Time
	// LastActor is the user who last modified the alarm state.
	LastActor *Actor
	// IsEnabled indicates whether the alarm is currently active.
	IsEnabled bool
}

// Clone returns a copy of the state to avoid leaking internal references.
func (s *State) Clone() *State {
	return &State{
		Timestamp: s.Timestamp,
		LastActor: s.LastActor.Clone(),
		IsEnabled: s.IsEnabled,
	}
}
