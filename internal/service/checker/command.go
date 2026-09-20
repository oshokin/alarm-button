package checker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/logger"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
	"github.com/oshokin/alarm-button/internal/proc"
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
	// PIDFile specifies the path to write current checker PID.
	PIDFile string
}

// DefaultPollInterval defines the fixed polling interval for alarm state checks.
const DefaultPollInterval = 5 * time.Second

// errShutdownInitiated indicates that a shutdown process has been initiated.
var errShutdownInitiated = errors.New("shutdown initiated")

// Run polls alarm state and optionally triggers shutdown when enabled.
// Loads configuration first to get timeout, uses default interval, and monitors alarm state.
//
//nolint:cyclop // Linear orchestration with explicit error handling is clearer as one flow.
func Run(ctx context.Context, opts *Options) error {
	ctx = logger.WithName(ctx, "alarm-checker")

	cfg, serverAddress, timeout, pollInterval, err := resolveCheckerRunSettings(ctx, opts)
	if err != nil {
		return err
	}

	pidFilePath, err := resolvePIDFilePath(opts.PIDFile)
	if err != nil {
		return err
	}

	err = proc.Write(pidFilePath, os.Getpid())
	if err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer func() {
		releaseErr := proc.RemoveIfOwned(pidFilePath, os.Getpid())
		if releaseErr != nil {
			logger.ErrorKV(ctx, "Failed to remove pid file", "path", pidFilePath, "error", releaseErr)
		}
	}()

	actor, err := common.DetectActor()
	if err != nil {
		return fmt.Errorf("detect actor: %w", err)
	}

	client, err := common.NewClient(serverAddress, cfg, common.WithCallTimeout(timeout))
	if err != nil {
		return fmt.Errorf("dial server: %w", err)
	}

	defer func() {
		_ = client.Close()
	}()

	logger.InfoKV(ctx, "Polling alarm state", "server_address", serverAddress, "interval", pollInterval.String())

	initialErr := checkState(ctx, client, actor, opts.Debug)
	if initialErr != nil {
		if errors.Is(initialErr, errShutdownInitiated) {
			logger.Info(ctx, "Shutdown initiated, exiting")
			return nil
		}

		logger.ErrorKV(ctx, "Initial state check failed", "error", initialErr)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info(ctx, "Context canceled, exiting")
			return nil
		case <-ticker.C:
			checkErr := checkState(ctx, client, actor, opts.Debug)
			if checkErr != nil {
				if errors.Is(checkErr, errShutdownInitiated) {
					logger.Info(ctx, "Shutdown initiated, exiting")
					return nil
				}

				logger.ErrorKV(ctx, "Check state failed", "error", checkErr)
			}
		}
	}
}

// resolveCheckerRunSettings resolves config, addresses, timeout, and polling interval.
func resolveCheckerRunSettings(
	ctx context.Context,
	opts *Options,
) (*config.Config, string, time.Duration, time.Duration, error) {
	if opts == nil {
		opts = &Options{}
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, "", 0, 0, fmt.Errorf("load configuration: %w", err)
	}

	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}

	serverAddress := cfg.ServerAddress
	if opts.ServerAddress != "" {
		serverAddress = opts.ServerAddress
	}

	if !cfg.TLS.Enabled && cfg.AllowInsecureRemote && !common.IsLoopbackAddress(serverAddress) {
		logger.WarnKV(
			ctx,
			"Insecure remote gRPC is explicitly enabled",
			"server_address",
			serverAddress,
		)
	}

	timeout := cfg.Timeout
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}

	return cfg, serverAddress, timeout, pollInterval, nil
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

	// Extract timestamp with fallback marker.
	timestamp := "<unknown>"
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

// resolvePIDFilePath resolves checker PID file path from override or executable path.
func resolvePIDFilePath(override string) (string, error) {
	if override != "" {
		if filepath.IsAbs(override) {
			return filepath.Clean(override), nil
		}

		absPath, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve pid file path: %w", err)
		}

		return filepath.Clean(absPath), nil
	}

	executablePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path for pid file: %w", err)
	}

	resolvedPath, err := filepath.EvalSymlinks(executablePath)
	if err != nil {
		resolvedPath = executablePath
	}

	pidPath, err := proc.DefaultPath(resolvedPath)
	if err != nil {
		return "", err
	}

	return filepath.Clean(pidPath), nil
}
