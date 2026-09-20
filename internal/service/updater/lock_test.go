package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAcquireUpdateLock_RecoversStaleLock verifies stale lock PID recovery.
func TestAcquireUpdateLock_RecoversStaleLock(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "alarm-button-update.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte(`{"pid":-1}`+"\n"), 0o600))

	lock, err := acquireUpdateLock(lockPath)
	require.NoError(t, err)
	require.NotNil(t, lock)
	require.NoError(t, lock.Release())
}

// TestAcquireUpdateLock_ActiveLockRejected verifies active lock is rejected.
func TestAcquireUpdateLock_ActiveLockRejected(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "alarm-button-update.lock")
	payload := fmt.Sprintf("{\"pid\":%d}\n", os.Getpid())
	require.NoError(t, os.WriteFile(lockPath, []byte(payload), 0o600))

	lock, err := acquireUpdateLock(lockPath)
	require.ErrorIs(t, err, errUpdateAlreadyRunning)
	require.Nil(t, lock)
}
