package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVersionStrings ensures Short and Full return non-empty consistent information.
func TestVersionStrings(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, Short())
	require.Contains(t, Full(), Short())
}
