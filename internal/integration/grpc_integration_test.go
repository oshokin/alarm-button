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
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
	"github.com/oshokin/alarm-button/internal/service/common"
	"github.com/oshokin/alarm-button/internal/service/server"
)

// startGRPC starts a gRPC server with temporary config and persistent state file.
// Returns a stop function to gracefully shutdown the server.
func startGRPC(t *testing.T, addr string, statePath string) (stop func()) {
	t.Helper()

	// Create cancellable context for server lifecycle.
	ctx, cancel := context.WithCancel(context.Background())
	cfgPath := filepath.Join(t.TempDir(), "settings.yaml")

	// Create temporary configuration file.
	require.NoError(
		t,
		config.Save(cfgPath, &config.Config{
			ServerAddress:      addr,
			ServerUpdateFolder: "http://127.0.0.1/",
			Timeout:            5 * time.Second,
		}),
	)

	// Start server in background goroutine.
	go func() {
		options := &server.Options{
			ConfigPath:    cfgPath,
			ListenAddress: "",
			StateFile:     statePath,
		}

		_ = server.Run(ctx, options) //nolint:errcheck // Test code needs simple net.Listen for port allocation.
	}()

	// Wait briefly for server to start listening.
	time.Sleep(150 * time.Millisecond)

	return func() {
		cancel()
		time.Sleep(100 * time.Millisecond)
	}
}

// TestGRPC_Roundtrip starts the real server and exercises client Set/Get with on-disk persistence.
func TestGRPC_Roundtrip(t *testing.T) {
	t.Parallel()

	// Reserve a free port for the test server.

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := l.Addr().String()
	_ = l.Close()

	// Setup temporary state file for persistence testing.
	statePath := filepath.Join(t.TempDir(), "state.json")

	// Start test gRPC server.
	stop := startGRPC(t, addr, statePath)
	defer stop()

	ctx := context.Background()

	// Connect to the test server with timeout.
	c, err := common.Dial(ctx, addr, common.WithCallTimeout(3*time.Second))
	require.NoError(t, err)

	defer func() {
		_ = c.Close()
	}()

	// Create test actor for audit logging.
	actor := &pb.SystemActor{
		Hostname: "test-hostname",
		Username: "test-user",
	}

	// Test initial state read - should succeed.
	_, err = c.GetAlarmState(ctx, actor)
	require.NoError(t, err)

	// Test state modification - enable alarm.
	_, err = c.SetAlarmState(ctx, actor, true)
	require.NoError(t, err)

	// Verify state was persisted correctly.
	got, err := c.GetAlarmState(ctx, actor)
	require.NoError(t, err)
	require.True(t, got.GetIsEnabled())

	// Verify state was persisted to disk.
	_, err = os.Stat(statePath)
	require.NoError(t, err)
}
