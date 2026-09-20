package proc

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefaultPath verifies PID file default location derivation.
func TestDefaultPath(t *testing.T) {
	t.Parallel()

	executablePath := filepath.Join("/opt/alarm-button", "alarm-server")
	if runtime.GOOS == "windows" {
		executablePath = filepath.Join(`C:\alarm-button`, "alarm-server.exe")
	}

	pidPath, err := DefaultPath(executablePath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(filepath.Dir(executablePath), "alarm-server.pid"), pidPath)
}

// TestWriteReadAndRemoveIfOwned verifies PID file lifecycle helpers.
func TestWriteReadAndRemoveIfOwned(t *testing.T) {
	t.Parallel()

	pidPath := filepath.Join(t.TempDir(), "alarm-server.pid")
	require.NoError(t, Write(pidPath, os.Getpid()))

	readPID, err := Read(pidPath)
	require.NoError(t, err)
	require.Equal(t, os.Getpid(), readPID)

	require.NoError(t, RemoveIfOwned(pidPath, os.Getpid()))

	_, err = os.Stat(pidPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}
