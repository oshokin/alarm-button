package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWriteFileAtomic verifies atomic write creates file with expected content.
func TestWriteFileAtomic(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "test.txt")
	data := []byte("hello")

	require.NoError(t, WriteFileAtomic(path, data, fs.FileMode(0o600)))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, data, got)
}

// TestWriteFileAtomic_Overwrite verifies atomic write replaces existing content.
func TestWriteFileAtomic_Overwrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))

	require.NoError(t, WriteFileAtomic(path, []byte("new"), fs.FileMode(0o600)))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte("new"), got)
}
