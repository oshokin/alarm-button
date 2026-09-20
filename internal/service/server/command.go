package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	api "github.com/oshokin/alarm-button/internal/api/grpc/alarm"
	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/logger"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
	"github.com/oshokin/alarm-button/internal/proc"
	repository "github.com/oshokin/alarm-button/internal/repository/state"
)

// defaultShutdownTimeout bounds graceful gRPC server shutdown.
const defaultShutdownTimeout = 5 * time.Second

// Server TLS and transport policy validation errors.
var (
	errTLSCAFileHasNoCertificates = errors.New("tls ca file contains no valid certificates")
	errInsecureNonLoopbackListen  = errors.New(
		"refusing non-loopback gRPC listen without tls; enable tls or set allow_insecure_remote=true",
	)
)

// Options controls the alarm-server process and configuration.
type Options struct {
	// ConfigPath specifies the path to settings YAML file.
	ConfigPath string
	// ListenAddress provides an optional listen address override for the gRPC server.
	ListenAddress string
	// StateFile specifies the path to persist alarm state JSON.
	StateFile string
	// PIDFile specifies the path to write current server PID.
	PIDFile string
	// Listener is optional and primarily intended for tests to avoid port races.
	Listener net.Listener
}

// Run starts the gRPC server and blocks until context is canceled or server stops.
//
//nolint:cyclop // Server orchestration keeps lifecycle concerns explicit in one place.
func Run(ctx context.Context, opts *Options) error {
	if opts == nil {
		opts = &Options{}
	}

	ctx = logger.WithName(ctx, "alarm-server")

	settings, err := config.Load(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	stateFile := settings.StateFile
	if opts.StateFile != "" {
		stateFile = opts.StateFile
	}

	releasePIDFile, err := registerPIDFile(ctx, opts.PIDFile)
	if err != nil {
		return err
	}
	defer releasePIDFile()

	listenAddress := effectiveListenAddress(settings, opts.ListenAddress)
	if err = validateServerTransportPolicy(ctx, settings, listenAddress); err != nil {
		return err
	}

	repo := repository.NewFileRepository(stateFile)

	svc, err := newService(ctx, repo)
	if err != nil {
		return fmt.Errorf("initialize service: %w", err)
	}

	lis, err := resolveListener(ctx, opts.Listener, listenAddress)
	if err != nil {
		return err
	}

	grpcServer, healthServer, err := buildGRPCServer(settings, svc)
	if err != nil {
		return err
	}

	logger.InfoKV(ctx, "Alarm server listening", "listen_address", lis.Addr().String(), "state_file", stateFile)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- grpcServer.Serve(lis)
	}()

	select {
	case serveErrValue := <-serveErr:
		if serveErrValue == nil || errors.Is(serveErrValue, grpc.ErrServerStopped) {
			return nil
		}

		return fmt.Errorf("serve gRPC: %w", serveErrValue)

	case <-ctx.Done():
		logger.Info(ctx, "Shutting down gRPC server")
		healthServer.SetServingStatus(pb.AlarmService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_NOT_SERVING)
		stopGRPCServer(grpcServer, defaultShutdownTimeout)

		serveErrValue := <-serveErr
		if serveErrValue == nil || errors.Is(serveErrValue, grpc.ErrServerStopped) {
			logger.Info(ctx, "gRPC server stopped")
			return nil
		}

		return fmt.Errorf("serve gRPC: %w", serveErrValue)
	}
}

// registerPIDFile writes process PID file and returns release callback.
func registerPIDFile(ctx context.Context, override string) (func(), error) {
	pidFilePath, err := resolvePIDFilePath(override)
	if err != nil {
		return nil, err
	}

	err = proc.Write(pidFilePath, os.Getpid())
	if err != nil {
		return nil, fmt.Errorf("write pid file: %w", err)
	}

	release := func() {
		releaseErr := proc.RemoveIfOwned(pidFilePath, os.Getpid())
		if releaseErr != nil {
			logger.ErrorKV(ctx, "Failed to remove pid file", "path", pidFilePath, "error", releaseErr)
		}
	}

	return release, nil
}

// resolveListener returns provided listener or creates one for listen address.
func resolveListener(ctx context.Context, provided net.Listener, listenAddress string) (net.Listener, error) {
	if provided != nil {
		return provided, nil
	}

	lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", listenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", listenAddress, err)
	}

	return lis, nil
}

// resolvePIDFilePath resolves absolute PID file path from override or executable.
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

// buildGRPCServer builds gRPC server with service and health registrations.
func buildGRPCServer(
	settings *config.Config,
	svc *service,
) (*grpc.Server, *health.Server, error) {
	grpcOptions := []grpc.ServerOption{
		grpc.UnaryInterceptor(loggingUnaryInterceptor),
	}

	if settings.TLS.Enabled {
		serverTLSConfig, tlsErr := buildServerTLSConfig(&settings.TLS)
		if tlsErr != nil {
			return nil, nil, fmt.Errorf("build server tls config: %w", tlsErr)
		}

		grpcOptions = append(grpcOptions, grpc.Creds(credentials.NewTLS(serverTLSConfig)))
	}

	grpcServer := grpc.NewServer(grpcOptions...)
	healthServer := health.NewServer()

	pb.RegisterAlarmServiceServer(grpcServer, api.NewServer(svc))
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus(pb.AlarmService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)

	return grpcServer, healthServer, nil
}

// effectiveListenAddress resolves final listen address from override/config/default.
func effectiveListenAddress(cfg *config.Config, override string) string {
	if override != "" {
		return override
	}

	if cfg.ListenAddress != "" {
		return cfg.ListenAddress
	}

	return config.DefaultListenAddress
}

// buildServerTLSConfig builds server TLS configuration from config file options.
func buildServerTLSConfig(tlsCfg *config.TLSConfig) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(tlsCfg.CertFile, tlsCfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load tls certificate: %w", err)
	}

	result := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

	if tlsCfg.CAFile == "" {
		return result, nil
	}

	caPEM, err := os.ReadFile(filepath.Clean(tlsCfg.CAFile))
	if err != nil {
		return nil, fmt.Errorf("read tls ca file: %w", err)
	}

	clientCAs := x509.NewCertPool()
	if ok := clientCAs.AppendCertsFromPEM(caPEM); !ok {
		return nil, errTLSCAFileHasNoCertificates
	}

	result.ClientCAs = clientCAs

	if tlsCfg.RequireClientCertificate {
		result.ClientAuth = tls.RequireAndVerifyClientCert
	} else {
		result.ClientAuth = tls.VerifyClientCertIfGiven
	}

	return result, nil
}

// stopGRPCServer performs graceful stop with bounded timeout fallback.
func stopGRPCServer(server *grpc.Server, timeout time.Duration) {
	stopped := make(chan struct{})

	go func() {
		server.GracefulStop()
		close(stopped)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-stopped:
	case <-timer.C:
		server.Stop()
		<-stopped
	}
}

// loggingUnaryInterceptor logs gRPC method, duration, and resulting status.
func loggingUnaryInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp any, err error) {
	started := time.Now()

	resp, err = handler(ctx, req)

	logger.InfoKV(
		ctx,
		"gRPC request completed",
		"method",
		info.FullMethod,
		"duration",
		time.Since(started),
		"status",
		status.Code(err).String(),
	)

	return resp, err
}

// validateServerTransportPolicy enforces TLS for non-loopback listeners by default.
func validateServerTransportPolicy(ctx context.Context, cfg *config.Config, listenAddress string) error {
	if cfg.TLS.Enabled || isLoopbackListenAddress(listenAddress) {
		return nil
	}

	if cfg.AllowInsecureRemote {
		logger.WarnKV(
			ctx,
			"Insecure non-loopback gRPC listener is explicitly enabled",
			"listen_address",
			listenAddress,
		)

		return nil
	}

	return errInsecureNonLoopbackListen
}

// isLoopbackListenAddress reports whether listen address is loopback-only.
func isLoopbackListenAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}

	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	// Empty host means "all interfaces", which is not loopback-only.
	if host == "" {
		return false
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}
