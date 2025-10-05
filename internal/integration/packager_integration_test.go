package integration

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/service/packager"
	upd "github.com/oshokin/alarm-button/internal/service/updater"
)

// TestPackager_WritesManifest generates a minimal manifest with placeholder files and verifies it exists.
func TestPackager_WritesManifest(t *testing.T) {
	// Setup test directory and change working directory.
	dir := t.TempDir()
	prev, _ := os.Getwd() //nolint:errcheck // Test code needs simple os.Getwd for directory change.

	t.Chdir(dir)

	t.Cleanup(func() {
		t.Chdir(prev)
	})

	// Start a real gRPC server so reachability check passes.
	addr := reservePort(t)
	statePath := filepath.Join(dir, "state.json")

	stop := startGRPC(t, addr, statePath)
	defer stop()

	// Create placeholder files expected by packager.
	for _, name := range upd.FilesWithChecksum() {
		f, err := os.Create(name)
		require.NoError(t, err)

		_ = f.Close()
	}

	// Run packager with timeout context.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	options := &packager.Options{
		// Ensure the settings file is one of checksummed files.
		ConfigPath:    config.DefaultConfigFilename,
		UpdateFolder:  "http://localhost/updates",
		ServerAddress: addr,
	}

	err := packager.Run(ctx, options)
	require.NoError(t, err)

	// Verify version manifest file was created.
	_, err = os.Stat(upd.VersionFilename)
	require.NoError(t, err)
}

// ReservePort returns address on a free TCP port and closes it.
func reservePort(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := l.Addr().String()
	_ = l.Close()

	return addr
}
