//go:build windows

package updater

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// helperBuildTimeout bounds fixture compilation for Windows executable replacement test.
const helperBuildTimeout = 2 * time.Minute

// TestApplyExecutable_RunningWindowsBinary verifies deferred replacement on locked executable.
func TestApplyExecutable_RunningWindowsBinary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	doneFile := filepath.Join(dir, "done")
	targetPath := filepath.Join(dir, "alarm-updater.exe")
	replacementPath := filepath.Join(dir, "replacement.exe")

	runningSource := filepath.Join(dir, "running.go")
	replacementSource := filepath.Join(dir, "replacement.go")
	require.NoError(t, os.WriteFile(runningSource, []byte(`package main
import (
	"os"
	"time"
)
func main() {
	doneFile := os.Getenv("ALARM_DONE_FILE")
	for {
		if _, err := os.Stat(doneFile); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
`), 0o600))
	require.NoError(t, os.WriteFile(replacementSource, []byte(`package main
import "fmt"
func main() { fmt.Println("updated") }
`), 0o600))

	buildHelperBinary(t, targetPath, runningSource)
	buildHelperBinary(t, replacementPath, replacementSource)

	process := exec.Command(targetPath)
	process.Env = append(os.Environ(), "ALARM_DONE_FILE="+doneFile)
	require.NoError(t, process.Start())

	t.Cleanup(func() {
		require.NoError(t, os.WriteFile(doneFile, []byte("done"), 0o600))
		_ = process.Wait()
	})

	expectedChecksum := checksumFileBase64(t, replacementPath)
	require.NoError(t, scheduleWindowsUpdaterReplacement(process.Process.Pid, replacementPath, targetPath))

	require.NoError(t, os.WriteFile(doneFile, []byte("done"), 0o600))
	require.NoError(t, process.Wait())

	require.Eventually(t, func() bool {
		actualChecksum, checksumErr := checksumFileBase64WithError(targetPath)
		if checksumErr != nil {
			return false
		}

		return actualChecksum == expectedChecksum
	}, 30*time.Second, 200*time.Millisecond)
}

// buildHelperBinary compiles fixture source into helper executable for test.
func buildHelperBinary(t *testing.T, outputPath string, sourcePath string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), helperBuildTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "build", "-o", outputPath, sourcePath)
	output, err := command.CombinedOutput()
	require.NoErrorf(t, err, "build helper binary failed: %s", string(output))
}

// checksumFileBase64 computes base64 SHA-512 checksum for file path.
func checksumFileBase64(t *testing.T, path string) string {
	t.Helper()

	checksum, err := checksumFileBase64WithError(path)
	require.NoError(t, err)

	return checksum
}

// checksumFileBase64WithError computes checksum and returns read/hash errors.
func checksumFileBase64WithError(path string) (string, error) {
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", err
	}

	sum := sha512.Sum512(content)

	return base64.StdEncoding.EncodeToString(sum[:]), nil
}
