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

// TestActorString verifies human-readable actor formatting.
func TestActorString(t *testing.T) {
	t.Parallel()

	require.Equal(t, nilEntityValue, (*Actor)(nil).String())
	require.Equal(t, "unknown@unknown", (&Actor{}).String())
	require.Equal(
		t,
		"o.shokin@alarm-pc",
		(&Actor{Username: "o.shokin", Hostname: "alarm-pc"}).String(),
	)
}

// TestStateString verifies human-readable state formatting.
func TestStateString(t *testing.T) {
	t.Parallel()

	timestamp := time.Unix(10, 250).UTC()
	state := &State{
		Timestamp: timestamp,
		LastActor: &Actor{
			Username: "alice",
			Hostname: "workstation-7",
		},
		IsEnabled: true,
	}

	require.Equal(
		t,
		"enabled=true actor=alice@workstation-7 timestamp=1970-01-01T00:00:10.00000025Z",
		state.String(),
	)
	require.Equal(t, nilEntityValue, (*State)(nil).String())
}
