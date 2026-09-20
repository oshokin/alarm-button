package packager

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGenerateSigningKey_RefusesOverwriteWithoutForce verifies safe default overwrite policy.
func TestGenerateSigningKey_RefusesOverwriteWithoutForce(t *testing.T) {
	t.Parallel()

	keyPath := filepath.Join(t.TempDir(), "signing-key.pem")
	_, err := GenerateSigningKey(keyPath, "2026-03", false)
	require.NoError(t, err)

	_, err = GenerateSigningKey(keyPath, "2026-03", false)
	require.ErrorIs(t, err, errSigningKeyAlreadyExists)
}

// TestGenerateSigningKey_ForceOverwriteReplacesFile verifies force overwrite behavior.
func TestGenerateSigningKey_ForceOverwriteReplacesFile(t *testing.T) {
	t.Parallel()

	keyPath := filepath.Join(t.TempDir(), "signing-key.pem")
	_, err := GenerateSigningKey(keyPath, "2026-03", false)
	require.NoError(t, err)

	originalContent, err := os.ReadFile(keyPath)
	require.NoError(t, err)

	_, err = GenerateSigningKey(keyPath, "2026-04", true)
	require.NoError(t, err)

	updatedContent, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	require.NotEqual(t, string(originalContent), string(updatedContent))
}

// TestGenerateSigningKey_ForceOverwriteSetsFileMode verifies permission hardening on overwrite.
func TestGenerateSigningKey_ForceOverwriteSetsFileMode(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Windows does not provide POSIX permission bit semantics for os.Chmod")
	}

	keyPath := filepath.Join(t.TempDir(), "signing-key.pem")
	require.NoError(t, os.WriteFile(keyPath, []byte("placeholder"), 0o644))

	_, err := GenerateSigningKey(keyPath, "2026-04", true)
	require.NoError(t, err)

	info, err := os.Stat(keyPath)
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
}
