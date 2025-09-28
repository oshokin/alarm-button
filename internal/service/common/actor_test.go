//nolint:revive,nolintlint // Package name "common" is intentional for shared helpers.
package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDetectActor ensures hostname and username are detected and non-empty.
func TestDetectActor(t *testing.T) {
	t.Parallel()

	a, err := DetectActor()
	require.NoError(t, err)
	require.NotEmpty(t, a.GetHostname())
	require.NotEmpty(t, a.GetUsername())
}
