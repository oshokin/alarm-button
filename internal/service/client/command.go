package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/logger"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
	"github.com/oshokin/alarm-button/internal/retry"
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

const (
	// UnknownValue is used as a fallback when data is not available.
	UnknownValue = "<unknown>"
	// defaultRetryInitialDelay is the first retry wait duration.
	defaultRetryInitialDelay = 250 * time.Millisecond
	// defaultRetryMaxDelay is the upper bound for retry waits.
	defaultRetryMaxDelay = 5 * time.Second
	// defaultRetryJitterDivisor adds up to 20% jitter to retry waits.
	defaultRetryJitterDivisor uint64 = 5
)

var (
	// errRetryableSetAlarmStateRPC marks transient gRPC failures as retryable.
	errRetryableSetAlarmStateRPC = errors.New("transient SetAlarmState RPC error")
	// errRetryableAlarmStateMismatch marks server/client state mismatch as retryable.
	errRetryableAlarmStateMismatch = errors.New("alarm state mismatch")
	// defaultSetAlarmStateRetryEngine is a reusable retry engine for alarm state changes.
	defaultSetAlarmStateRetryEngine = mustNewDefaultRetryEngine()
)

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

	actor, err := common.DetectActor()
	if err != nil {
		return fmt.Errorf("detect actor: %w", err)
	}

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
	return executeWithRetry(ctx, client, actor, opts, defaultSetAlarmStateRetryEngine)
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

	if !cfg.TLS.Enabled && cfg.AllowInsecureRemote && !common.IsLoopbackAddress(serverAddress) {
		logger.WarnKV(
			ctx,
			"Insecure remote gRPC is explicitly enabled",
			"server_address",
			serverAddress,
		)
	}

	// Connect to alarm server with timeout from configuration.
	client, err := common.NewClient(serverAddress, cfg, common.WithCallTimeout(cfg.Timeout))
	if err != nil {
		return nil, "", err
	}

	return client, serverAddress, nil
}

// executeWithRetry handles the retry logic for alarm state changes.
func executeWithRetry(
	ctx context.Context,
	client *common.Client,
	actor *pb.SystemActor,
	opts *Options,
	retryEngine *retry.Engine,
) error {
	return retryEngine.Run(
		ctx,
		&retry.Request{
			Operation: func(operationCtx context.Context) error {
				return attemptAlarmStateChange(operationCtx, client, actor, opts)
			},
			OnRetry: logRetryAttempt,
		},
	)
}

// defaultRetryConfig builds retry engine config for SetAlarmState operations.
func defaultRetryConfig() *retry.EngineConfig {
	return &retry.EngineConfig{
		MaxRetries: 0,
		DelayPolicy: retry.NewExponentialBoundedJitterPolicy(
			defaultRetryInitialDelay,
			defaultRetryMaxDelay,
			defaultRetryJitterDivisor,
		),
		IsRetryable: isRetryableAlarmStateError,
	}
}

// mustNewDefaultRetryEngine builds the static retry engine and panics on invalid static config.
func mustNewDefaultRetryEngine() *retry.Engine {
	retryEngine, err := retry.NewEngine(defaultRetryConfig())
	if err != nil {
		panic(fmt.Sprintf("build default alarm-state retry engine: %v", err))
	}

	return retryEngine
}

// logRetryAttempt logs retry scheduling details.
func logRetryAttempt(ctx context.Context, info *retry.AttemptInfo) {
	logger.WarnKV(
		ctx,
		"SetAlarmState attempt did not converge; retrying",
		"retry",
		info.Retry,
		"delay",
		info.Delay,
		"error",
		info.Err,
	)
}

// attemptAlarmStateChange tries once to change alarm state.
func attemptAlarmStateChange(
	ctx context.Context,
	client *common.Client,
	actor *pb.SystemActor,
	opts *Options,
) error {
	// Request state change from server.
	resp, err := client.SetAlarmState(ctx, actor, opts.DesiredState)
	if err != nil {
		if isRetryableRPCError(err) {
			return fmt.Errorf("%w: %w", errRetryableSetAlarmStateRPC, err)
		}

		return fmt.Errorf("set alarm state: %w", err)
	}

	// Check if server confirmed the desired state change.
	if resp != nil && resp.GetIsEnabled() == opts.DesiredState {
		logger.Infof(ctx, "Alarm updated: %s", formatState(resp))

		// Handle shutdown if alarm is being enabled and not in debug mode.
		return handleShutdownIfNeeded(ctx, opts)
	}

	// Server responded but state mismatch, continue retrying.
	return errRetryableAlarmStateMismatch
}

// isRetryableAlarmStateError reports retry eligibility for SetAlarmState operation errors.
func isRetryableAlarmStateError(err error) bool {
	return errors.Is(err, errRetryableSetAlarmStateRPC) || errors.Is(err, errRetryableAlarmStateMismatch)
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

// isRetryableRPCError reports whether gRPC error should trigger retry loop.
func isRetryableRPCError(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}
