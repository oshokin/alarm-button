package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVersionStrings verifies combined short and full version rendering.
func TestVersionStrings(t *testing.T) {
	setBuildInfoForTest(t, "1.2.3", "abc123")
	require.Equal(t, "1.2.3", Short())
	require.Equal(t, "version: 1.2.3, commit: abc123", Full())
}

// TestShort verifies short version output.
func TestShort(t *testing.T) {
	setBuildInfoForTest(t, "2.1.3", "none")
	require.Equal(t, "2.1.3", Short())
}

// TestFull verifies full version output with commit metadata.
func TestFull(t *testing.T) {
	setBuildInfoForTest(t, "2.1.3", "abc123")
	require.Equal(t, "version: 2.1.3, commit: abc123", Full())
}

// setBuildInfoForTest temporarily overrides build metadata for test assertions.
func setBuildInfoForTest(t *testing.T, version string, commit string) {
	t.Helper()

	oldVersion := Version
	oldCommit := Commit

	Version = version
	Commit = commit

	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
	})
}
