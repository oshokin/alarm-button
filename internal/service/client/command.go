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

// Run attempts to set alarm state with retry logic until success or cancellation.
//
//nolint:cyclop,funlen // Complex business logic requires multiple conditional paths.
func Run(ctx context.Context, opts *Options) error {
	// Set context with logger name for tracking.
	ctx = logger.WithName(ctx, "alarm-button-on/off")

	// Load settings from configuration file.
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	// Use server address from options if provided, otherwise use config.
	serverAddress := cfg.ServerAddress
	if opts.ServerAddress != "" {
		serverAddress = opts.ServerAddress
	}

	// Identify current user and hostname for audit logging.
	actor, err := common.DetectActor()
	if err != nil {
		return err
	}

	// Connect to alarm server with timeout from config.
	client, err := common.Dial(ctx, serverAddress, common.WithCallTimeout(cfg.Timeout))
	if err != nil {
		return err
	}

	// Close connection on function exit.
	defer func() {
		_ = client.Close()
	}()

	logger.InfoKV(
		ctx,
		"Pushing desired alarm state",
		"server_address",
		serverAddress,
		"desired_state",
		opts.DesiredState,
	)

	// attempt tries once to change alarm state, returns (completed, error).
	attempt := func() (bool, error) {
		// Request state change from server.
		resp, err := client.SetAlarmState(ctx, actor, opts.DesiredState)
		if err != nil {
			// Log error but continue retrying for transient failures.
			logger.ErrorKV(ctx, "SetAlarmState failed", "error", err)
			return false, nil
		}

		// Check if server confirmed the desired state change.
		if resp != nil && resp.GetIsEnabled() == opts.DesiredState {
			logger.Infof(ctx, "Alarm updated: %s", formatState(resp))

			// Shutdown machine after enabling alarm unless in debug mode.
			if opts.DesiredState && !opts.Debug {
				logger.Info(ctx, "Triggering local shutdown...")

				if err := power.Shutdown(ctx); err != nil {
					return false, fmt.Errorf("shutdown: %w", err)
				}
			}

			return true, nil
		}

		// Server responded but state mismatch, continue retrying.
		return false, nil
	}

	// Attempt immediately before starting retry loop.
	if done, err := attempt(); err != nil {
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
			done, err := attempt()
			if err != nil {
				return err
			}

			if done {
				return nil
			}
		}
	}
}

// formatState converts alarm state response to readable log message.
func formatState(state *pb.AlarmStateResponse) string {
	if state == nil {
		return "<nil state>"
	}

	// Extract timestamp with fallback for missing data.
	timestamp := "<unknown>"
	if t := state.GetTimestamp(); t != nil {
		timestamp = t.AsTime().Format(time.RFC3339)
	}

	// Format actor as username@hostname with fallback.
	actor := "<unknown>"
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
