package alarm

import (
	"fmt"
	"time"
)

// Fallback string markers for entity rendering.
const (
	unknownActorValue = "unknown"
	nilEntityValue    = "<nil>"
)

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

// String returns stable human-readable actor identity.
func (a *Actor) String() string {
	if a == nil {
		return nilEntityValue
	}

	username := a.Username
	if username == "" {
		username = unknownActorValue
	}

	hostname := a.Hostname
	if hostname == "" {
		hostname = unknownActorValue
	}

	return username + "@" + hostname
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

// String returns stable human-readable alarm state.
func (s *State) String() string {
	if s == nil {
		return nilEntityValue
	}

	return fmt.Sprintf(
		"enabled=%t actor=%s timestamp=%s",
		s.IsEnabled,
		s.LastActor,
		s.Timestamp.UTC().Format(time.RFC3339Nano),
	)
}
