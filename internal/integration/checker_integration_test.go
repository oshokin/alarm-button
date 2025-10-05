package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/oshokin/alarm-button/internal/config"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
	"github.com/oshokin/alarm-button/internal/service/checker"
	"github.com/oshokin/alarm-button/internal/service/common"
)

// TestChecker_PollsAndReturnsOnCancel runs the checker against a live server in Debug mode and cancels it.
func TestChecker_PollsAndReturnsOnCancel(t *testing.T) {
	t.Parallel()

	// Setup test environment with server and temporary state.
	addr := reservePort(t)
	statePath := filepath.Join(t.TempDir(), "state.json")

	stop := startGRPC(t, addr, statePath)
	defer stop()

	ctx := context.Background()

	// Connect to the test server.
	c, err := common.Dial(ctx, addr)
	require.NoError(t, err)

	defer func() {
		_ = c.Close()
	}()

	// Enable alarm state so checker would attempt shutdown, but we'll set Debug=true.
	actor := &pb.SystemActor{
		Hostname: "test-host",
		Username: "test-user",
	}

	_, err = c.SetAlarmState(ctx, actor, true)
	require.NoError(t, err)

	// Setup cancellable context for checker.
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	// Create temporary config file for checker.
	cfgPath := filepath.Join(t.TempDir(), "checker-settings.yaml")
	err = config.Save(cfgPath, &config.Config{
		ServerAddress: addr,
		Timeout:       1 * time.Second,
	})
	require.NoError(t, err)

	// Start checker in debug mode (won't shutdown).
	go func() {
		options := &checker.Options{
			ConfigPath:    cfgPath,
			ServerAddress: addr, // Override config address
			PollInterval:  50 * time.Millisecond,
			Debug:         true,
		}

		done <- checker.Run(runCtx, options)
	}()

	// Wait for checker to start polling, then cancel.
	time.Sleep(120 * time.Millisecond)
	cancel()

	// Verify checker exits cleanly on cancellation.
	err = <-done
	require.NoError(t, err)
}
