package alarm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestActorClone verifies that Clone returns a deep copy and handles nil safely.
func TestActorClone(t *testing.T) {
	t.Parallel()
	require.Nil(t, (*Actor)(nil).Clone())

	a := &Actor{
		Hostname: "Oleg Shokin",
		Username: "o.shokin",
	}

	b := a.Clone()

	require.Equal(t, a, b)
	require.NotSame(t, a, b)
}

// TestStateClone verifies that State.Clone copies fields and deep-copies LastActor.
func TestStateClone(t *testing.T) {
	t.Parallel()

	ts := time.Now().UTC().Truncate(time.Second)
	s := State{
		Timestamp: ts,
		LastActor: &Actor{
			Hostname: "Oleg Shokin",
			Username: "o.shokin",
		},
		IsEnabled: true,
	}

	c := s.Clone()
	require.Equal(t, s.Timestamp, c.Timestamp)
	require.Equal(t, s.IsEnabled, c.IsEnabled)
	require.Equal(t, s.LastActor, c.LastActor)

	// Ensure actor pointer is cloned.
	require.NotSame(t, s.LastActor, c.LastActor)
}
