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

// TestGRPC_Roundtrip starts the real server and exercises client Set/Get with on-disk persistence.
func TestGRPC_Roundtrip(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "state.json")

	addr, stop := startGRPC(t, statePath)
	defer stop()

	ctx := context.Background()
	client, err := common.NewClient(addr, &config.Config{}, common.WithCallTimeout(3*time.Second))
	require.NoError(t, err)

	defer func() { _ = client.Close() }()

	actor := &pb.SystemActor{
		Hostname: "test-hostname",
		Username: "test-user",
	}

	_, err = client.GetAlarmState(ctx, actor)
	require.NoError(t, err)

	_, err = client.SetAlarmState(ctx, actor, true)
	require.NoError(t, err)

	got, err := client.GetAlarmState(ctx, actor)
	require.NoError(t, err)
	require.True(t, got.GetIsEnabled())

	_, err = os.Stat(statePath)
	require.NoError(t, err)
}

// startGRPC starts test gRPC server and returns address with stop callback.
func startGRPC(t *testing.T, statePath string) (address string, stop func()) {
	t.Helper()

	lc := net.ListenConfig{}
	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	address = lis.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	cfgPath := filepath.Join(t.TempDir(), "settings.yaml")

	require.NoError(
		t,
		config.Save(cfgPath, &config.Config{
			ServerAddress:      address,
			ListenAddress:      address,
			ServerUpdateFolder: "http://127.0.0.1/",
			Timeout:            5 * time.Second,
		}),
	)

	done := make(chan error, 1)
	go func() {
		done <- server.Run(ctx, &server.Options{
			ConfigPath: cfgPath,
			StateFile:  statePath,
			Listener:   lis,
		})
	}()

	require.Eventually(
		t,
		func() bool {
			client, dialErr := common.NewClient(address, &config.Config{}, common.WithCallTimeout(150*time.Millisecond))
			if dialErr != nil {
				return false
			}
			defer func() { _ = client.Close() }()

			_, requestErr := client.GetAlarmState(
				context.Background(),
				&pb.SystemActor{Hostname: "health", Username: "health"},
			)

			return requestErr == nil
		},
		time.Second,
		10*time.Millisecond,
	)

	stop = func() {
		cancel()

		select {
		case runErr := <-done:
			require.NoError(t, runErr)
		case <-time.After(time.Second):
			t.Fatal("server did not stop within timeout")
		}
	}

	return address, stop
}
