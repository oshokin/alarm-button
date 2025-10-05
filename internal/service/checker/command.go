package checker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/logger"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
	"github.com/oshokin/alarm-button/internal/service/common"
	"github.com/oshokin/alarm-button/internal/service/power"
)

// Options controls the checker polling behavior and configuration.
type Options struct {
	// ConfigPath specifies the path to the settings YAML file.
	ConfigPath string
	// ServerAddress provides an optional gRPC server address override.
	ServerAddress string
	// PollInterval defines the interval between alarm state checks.
	PollInterval time.Duration
	// Timeout specifies the per-RPC timeout duration.
	Timeout time.Duration
	// Debug prevents shutdown when the alarm is enabled for testing purposes.
	Debug bool
}

// DefaultPollInterval defines the fixed polling interval for alarm state checks.
const DefaultPollInterval = 5 * time.Second

// errShutdownInitiated indicates that a shutdown process has been initiated.
var errShutdownInitiated = errors.New("shutdown initiated")

// Run polls alarm state and optionally triggers shutdown when enabled.
// Loads configuration first to get timeout, uses default interval, and monitors alarm state.
//
//nolint:cyclop // Flow is straightforward and readable; splitting would reduce clarity.
func Run(ctx context.Context, opts *Options) error {
	// Set context with logger name for tracking.
	ctx = logger.WithName(ctx, "alarm-checker")

	// Load settings from configuration file.
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// Use default polling interval as it's not user-configurable.
	if opts.PollInterval <= 0 {
		opts.PollInterval = DefaultPollInterval
	}

	// Determine server address: command line argument overrides config.
	serverAddress := cfg.ServerAddress
	if opts.ServerAddress != "" {
		serverAddress = opts.ServerAddress
	}

	// Detect current system actor for audit logging.
	actor, err := common.DetectActor()
	if err != nil {
		return fmt.Errorf("detect actor: %w", err)
	}

	// Establish gRPC connection with timeout from configuration.
	client, err := common.Dial(ctx, serverAddress, common.WithCallTimeout(cfg.Timeout))
	if err != nil {
		return fmt.Errorf("dial server: %w", err)
	}

	// Ensure connection cleanup on function exit.
	defer func() {
		_ = client.Close()
	}()

	logger.InfoKV(ctx, "Polling alarm state", "server_address", serverAddress, "interval", opts.PollInterval.String())

	// Setup polling ticker with fixed interval.
	ticker := time.NewTicker(opts.PollInterval)
	defer ticker.Stop()

	// Main polling loop until context cancellation or shutdown.
	for {
		select {
		case <-ctx.Done():
			logger.Info(ctx, "Context canceled, exiting")
			return nil
		case <-ticker.C:
			// Check alarm state and handle shutdown if needed.
			if err = checkState(ctx, client, actor, opts.Debug); err != nil {
				if errors.Is(err, errShutdownInitiated) {
					logger.Info(ctx, "Shutdown initiated, exiting")
					return nil
				}

				logger.ErrorKV(ctx, "Check state failed", "error", err)
			}
		}
	}
}

// checkState retrieves and processes the current alarm state from the server.
// Logs alarm status and timestamp, initiates shutdown if alarm is enabled and debug is off.
// Returns errShutdownInitiated when shutdown is triggered, or error on failure.
func checkState(ctx context.Context, client *common.Client, actor *pb.SystemActor, debug bool) error {
	// Request current alarm state from server.
	state, err := client.GetAlarmState(ctx, actor)
	if err != nil {
		return err
	}

	// Format alarm status for logging.
	status := "disabled"
	if state.GetIsEnabled() {
		status = "enabled"
	}

	// Extract timestamp with fallback to current time.
	timestamp := time.Now().Format(time.RFC3339)
	if ts := state.GetTimestamp(); ts != nil {
		timestamp = ts.AsTime().Format(time.RFC3339)
	}

	logger.Infof(ctx, "Alarm state: %s at %s", status, timestamp)

	// Process alarm enabled state.
	if !state.GetIsEnabled() {
		return nil
	}

	if debug {
		logger.Info(ctx, "Alarm enabled but debug mode prevents shutdown")
		return nil
	}

	logger.Info(ctx, "Alarm enabled, initiating shutdown")

	// Trigger system shutdown.
	if err = power.Shutdown(ctx); err != nil {
		return err
	}

	return errShutdownInitiated
}
