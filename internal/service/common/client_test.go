//nolint:revive,nolintlint // Package name "common" is intentional for shared helpers.
package common

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestDial_ValidatesAddress verifies that Dial rejects empty addresses.
func TestDial_ValidatesAddress(t *testing.T) {
	t.Parallel()

	c, err := Dial(context.Background(), "")
	require.Error(t, err)
	require.Nil(t, c)
}

// TestClient_callContext checks timeout vs cancel-only behavior of callContext.
func TestClient_callContext(t *testing.T) {
	t.Parallel()

	c := &Client{
		callTimeout: 0,
	}

	ctx, cancel := c.callContext(context.Background())
	cancel()

	require.NotNil(t, ctx)

	c.callTimeout = 10 * time.Millisecond

	ctx, cancel = c.callContext(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(10*time.Millisecond), deadline, 30*time.Millisecond)
}

// TestSetAlarmState_NilActor asserts that a nil actor is rejected by the client.
func TestSetAlarmState_NilActor(t *testing.T) {
	t.Parallel()

	c := new(Client)

	_, err := c.SetAlarmState(context.Background(), nil, true)
	require.Error(t, err)
}
