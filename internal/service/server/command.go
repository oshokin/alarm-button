package server

import (
	"context"
	"errors"
	"fmt"
	"net"

	"google.golang.org/grpc"

	api "github.com/oshokin/alarm-button/internal/api/grpc/alarm"
	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/logger"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
	repository "github.com/oshokin/alarm-button/internal/repository/state"
)

// Options controls the alarm-server process and configuration.
type Options struct {
	// ConfigPath specifies the path to settings YAML file.
	ConfigPath string
	// ListenAddress provides an optional listen address override for the gRPC server.
	ListenAddress string
	// StateFile specifies the path to persist alarm state JSON.
	StateFile string
}

// ErrNoServerAddress indicates missing server configuration.
var ErrNoServerAddress = errors.New("no server address configured")

// Run starts the gRPC server and blocks until context is canceled or server stops.
// Loads configuration first, then determines listen address from config or override.
func Run(ctx context.Context, opts *Options) error {
	// Set context with logger name for tracking.
	ctx = logger.WithName(ctx, "alarm-server")

	// Load configuration first to get server settings.
	settings, err := config.Load(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	// Use StateFile from config unless overridden by command line option.
	stateFile := settings.StateFile
	if opts.StateFile != "" {
		stateFile = opts.StateFile
	}

	// Determine listen address: CLI argument overrides config port extraction.
	listenAddress, err := resolveListenAddress(settings.ServerAddress, opts.ListenAddress)
	if err != nil {
		return fmt.Errorf("resolve listen address: %w", err)
	}

	// Initialize state repository for alarm persistence.
	repo := repository.NewFileRepository(stateFile)

	// Create alarm service with state management.
	svc, err := newService(ctx, repo)
	if err != nil {
		return fmt.Errorf("initialise service: %w", err)
	}

	// Setup TCP listener for gRPC server.
	lc := net.ListenConfig{}

	lis, err := lc.Listen(ctx, "tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", listenAddress, err)
	}

	// Create and configure gRPC server with alarm service.
	grpcServer := grpc.NewServer()
	pb.RegisterAlarmServiceServer(grpcServer, api.NewServer(svc))

	logger.InfoKV(ctx, "Alarm server listening", "listen_address", listenAddress, "state_file", stateFile)

	// Done channel is closed after GracefulStop finishes to ensure we block
	// until the server fully stops before returning.
	done := make(chan struct{})

	go func() {
		<-ctx.Done()
		logger.Info(ctx, "Shutting down gRPC server")
		grpcServer.GracefulStop()
		close(done)
	}()

	if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return fmt.Errorf("serve gRPC: %w", err)
	}

	<-done
	logger.Info(ctx, "GRPC server stopped")

	return nil
}

// resolveListenAddress determines the listen address for the gRPC server.
// If override is provided, uses it directly. Otherwise extracts port from configAddr.
// Returns appropriate listen address (e.g., ":8080" for port-only binding).
func resolveListenAddress(configAddr, override string) (string, error) {
	// Use override address if provided (e.g., ":9090", "0.0.0.0:8080").
	if override != "" {
		return override, nil
	}

	// Extract port from config address (e.g., "server.example.com:8080" -> ":8080").
	if configAddr == "" {
		return "", ErrNoServerAddress
	}

	// Parse the address to extract port.
	_, port, err := net.SplitHostPort(configAddr)
	if err != nil {
		return "", fmt.Errorf("invalid server address format %q: %w", configAddr, err)
	}

	// Return port-only listen address to bind on all interfaces.
	return ":" + port, nil
}
