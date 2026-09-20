package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oshokin/alarm-button/internal/config"
)

// TestValidateServerTransportPolicy verifies TLS policy for loopback/remote listeners.
func TestValidateServerTransportPolicy(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}

	cfg.TLS.Enabled = false
	cfg.AllowInsecureRemote = false
	require.NoError(t, validateServerTransportPolicy(context.Background(), cfg, "127.0.0.1:50051"))

	err := validateServerTransportPolicy(context.Background(), cfg, "0.0.0.0:50051")
	require.Error(t, err)

	cfg.AllowInsecureRemote = true
	require.NoError(t, validateServerTransportPolicy(context.Background(), cfg, "0.0.0.0:50051"))

	cfg.TLS.Enabled = true
	cfg.AllowInsecureRemote = false
	require.NoError(t, validateServerTransportPolicy(context.Background(), cfg, "0.0.0.0:50051"))
}
