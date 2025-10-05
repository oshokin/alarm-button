package client

import (
	"context"
	"fmt"
	"time"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/logger"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
	"github.com/oshokin/alarm-button/internal/service/common"
	"github.com/oshokin/alarm-button/internal/service/power"
)

// Options configures alarm client behavior for state change operations.
type Options struct {
	// ConfigPath to YAML settings file, defaults to standard filename if empty.
	ConfigPath string

	// ServerAddress overrides server address from config when specified.
	ServerAddress string

	// DesiredState represents target alarm state (true=on, false=off).
	DesiredState bool

	// Debug prevents local shutdown when true, used for testing alarm-on.
	Debug bool
}

// DefaultPushInterval defines retry delay when pushing alarm state to server.
const defaultPushInterval = 1 * time.Second

// UnknownValue is used as a fallback when data is not available.
const UnknownValue = "<unknown>"

// Run attempts to set alarm state with retry logic until success or cancellation.
func Run(ctx context.Context, opts *Options) error {
	// Set context with logger name for tracking.
	ctx = logger.WithName(ctx, "alarm-button-on/off")

	// Setup client connection and configuration.
	client, serverAddress, err := setupClient(ctx, opts)
	if err != nil {
		return err
	}

	defer func() {
		_ = client.Close()
	}()

	// Log the operation start.
	logger.InfoKV(
		ctx,
		"Pushing desired alarm state",
		"server_address",
		serverAddress,
		"desired_state",
		opts.DesiredState,
	)

	// Execute the alarm state change with retry logic.
	return executeWithRetry(ctx, client, opts)
}

// setupClient handles client configuration and connection setup.
func setupClient(ctx context.Context, opts *Options) (*common.Client, string, error) {
	// Load settings from configuration file.
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, "", err
	}

	// Use server address from options if provided, otherwise use config.
	serverAddress := cfg.ServerAddress
	if opts.ServerAddress != "" {
		serverAddress = opts.ServerAddress
	}

	// Connect to alarm server with timeout from config.
	client, err := common.Dial(ctx, serverAddress, common.WithCallTimeout(cfg.Timeout))
	if err != nil {
		return nil, "", err
	}

	return client, serverAddress, nil
}

// executeWithRetry handles the retry logic for alarm state changes.
func executeWithRetry(ctx context.Context, client *common.Client, opts *Options) error {
	// Attempt immediately before starting retry loop.
	if done, err := attemptAlarmStateChange(ctx, client, opts); err != nil {
		return err
	} else if done {
		return nil
	}

	// Setup retry timer for subsequent attempts.
	ticker := time.NewTicker(defaultPushInterval)
	defer ticker.Stop()

	// Retry loop until success or cancellation.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			done, err := attemptAlarmStateChange(ctx, client, opts)
			if err != nil {
				return err
			}

			if done {
				return nil
			}
		}
	}
}

// attemptAlarmStateChange tries once to change alarm state, returns (completed, error).
func attemptAlarmStateChange(ctx context.Context, client *common.Client, opts *Options) (bool, error) {
	// Identify current user and hostname for audit logging.
	actor, err := common.DetectActor()
	if err != nil {
		return false, err
	}

	// Request state change from server.
	var resp *pb.AlarmStateResponse

	resp, err = client.SetAlarmState(ctx, actor, opts.DesiredState)
	if err != nil {
		// Log error but continue retrying for transient failures.
		logger.ErrorKV(ctx, "SetAlarmState failed", "error", err)
		return false, nil
	}

	// Check if server confirmed the desired state change.
	if resp != nil && resp.GetIsEnabled() == opts.DesiredState {
		logger.Infof(ctx, "Alarm updated: %s", formatState(resp))

		// Handle shutdown if alarm is being enabled and not in debug mode.
		if err = handleShutdownIfNeeded(ctx, opts); err != nil {
			return false, err
		}

		return true, nil
	}

	// Server responded but state mismatch, continue retrying.
	return false, nil
}

// handleShutdownIfNeeded triggers shutdown if alarm is being enabled and not in debug mode.
func handleShutdownIfNeeded(ctx context.Context, opts *Options) error {
	if opts.DesiredState && !opts.Debug {
		logger.Info(ctx, "Triggering local shutdown...")

		if err := power.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
	}

	return nil
}

// formatState converts alarm state response to readable log message.
func formatState(state *pb.AlarmStateResponse) string {
	if state == nil {
		return "<nil state>"
	}

	// Extract timestamp with fallback for missing data.
	timestamp := UnknownValue
	if t := state.GetTimestamp(); t != nil {
		timestamp = t.AsTime().Format(time.RFC3339)
	}

	// Format actor as username@hostname with fallback.
	actor := UnknownValue
	if state.GetLastActor() != nil {
		actor = fmt.Sprintf("%s@%s", state.GetLastActor().GetUsername(), state.GetLastActor().GetHostname())
	}

	// Convert boolean state to readable string.
	status := "disabled"
	if state.GetIsEnabled() {
		status = "enabled"
	}

	return fmt.Sprintf("%s by %s (%s)", status, actor, timestamp)
}
