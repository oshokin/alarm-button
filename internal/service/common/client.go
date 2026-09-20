//nolint:revive,nolintlint // Package name "common" is intentional for shared helpers.
package common

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/oshokin/alarm-button/internal/config"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
)

// Client wraps the gRPC AlarmService client with convenience helpers.
type Client struct {
	// conn is the underlying gRPC connection to the alarm server.
	conn *grpc.ClientConn
	// api is the generated AlarmService client interface.
	api pb.AlarmServiceClient

	// callTimeout is the default timeout for individual RPC calls.
	callTimeout time.Duration
}

// Option configures client behavior.
type Option func(*Client)

// WithCallTimeout sets a default timeout for service calls.
func WithCallTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.callTimeout = timeout
		}
	}
}

// Client transport and request validation errors.
var (
	// errAddressRequired is returned when a required address value is missing.
	errAddressRequired = errors.New("address must be provided")
	// errActorRequired is returned when an actor is not provided but is required for the operation.
	errActorRequired = errors.New("actor must be provided")
	// errInsecureRemoteWithoutTLS is returned when insecure transport is used for non-loopback target.
	errInsecureRemoteWithoutTLS = errors.New(
		"refusing insecure non-loopback gRPC connection; enable tls or set allow_insecure_remote=true",
	)
	// errTLSCAFileHasNoCerts is returned when PEM CA file does not contain any valid certificates.
	errTLSCAFileHasNoCerts = errors.New("tls ca file contains no valid certificates")
	// errServiceNotServing indicates unhealthy remote endpoint.
	errServiceNotServing = errors.New("service is not serving")
)

// NewClient establishes a gRPC connection to the alarm server.
func NewClient(address string, cfg *config.Config, opts ...Option) (*Client, error) {
	if address == "" {
		return nil, errAddressRequired
	}

	transportCredentials, err := clientTransportCredentials(address, cfg)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(transportCredentials))
	if err != nil {
		return nil, fmt.Errorf("dial alarm server: %w", err)
	}

	client := &Client{
		conn:        conn,
		api:         pb.NewAlarmServiceClient(conn),
		callTimeout: config.DefaultTimeout,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client, nil
}

// Close releases the underlying gRPC connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

// GetAlarmState retrieves the current alarm state.
func (c *Client) GetAlarmState(ctx context.Context, actor *pb.SystemActor) (*pb.AlarmStateResponse, error) {
	callCtx, cancel := c.callContext(ctx)
	defer cancel()

	resp, err := c.api.GetAlarmState(callCtx, &pb.GetAlarmStateRequest{RequestingActor: actor})
	if err != nil {
		return nil, fmt.Errorf("get alarm state: %w", err)
	}

	return resp, nil
}

// SetAlarmState updates the remote alarm state.
func (c *Client) SetAlarmState(
	ctx context.Context,
	actor *pb.SystemActor,
	isEnabled bool,
) (*pb.AlarmStateResponse, error) {
	if actor == nil {
		return nil, errActorRequired
	}

	callCtx, cancel := c.callContext(ctx)
	defer cancel()

	request := &pb.SetAlarmStateRequest{
		Actor:     actor,
		IsEnabled: isEnabled,
	}

	response, err := c.api.SetAlarmState(callCtx, request)
	if err != nil {
		return nil, fmt.Errorf("set alarm state: %w", err)
	}

	return response, nil
}

// CheckHealth validates serving status via standard gRPC health endpoint.
func (c *Client) CheckHealth(ctx context.Context, service string) error {
	callCtx, cancel := c.callContext(ctx)
	defer cancel()

	response, err := healthpb.NewHealthClient(c.conn).Check(callCtx, &healthpb.HealthCheckRequest{
		Service: service,
	})
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}

	if response.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("%w: %s", errServiceNotServing, response.GetStatus().String())
	}

	return nil
}

// callContext returns a context with the client's call timeout if configured,
// otherwise a cancellable child context without a deadline.
func (c *Client) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.callTimeout <= 0 {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, c.callTimeout)
}

// IsLoopbackAddress reports whether target address resolves to loopback host.
func IsLoopbackAddress(address string) bool {
	return isLoopbackTarget(address)
}

//nolint:ireturn // grpc API requires credentials.TransportCredentials interface.
func clientTransportCredentials(address string, cfg *config.Config) (credentials.TransportCredentials, error) {
	if cfg != nil && cfg.TLS.Enabled {
		tlsConfig, err := buildClientTLSConfig(&cfg.TLS)
		if err != nil {
			return nil, err
		}

		return credentials.NewTLS(tlsConfig), nil
	}

	if isLoopbackTarget(address) {
		return insecure.NewCredentials(), nil
	}

	if cfg != nil && cfg.AllowInsecureRemote {
		return insecure.NewCredentials(), nil
	}

	return nil, errInsecureRemoteWithoutTLS
}

// buildClientTLSConfig builds TLS config for outbound gRPC connections.
func buildClientTLSConfig(tlsCfg *config.TLSConfig) (*tls.Config, error) {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}

	if tlsCfg.CAFile != "" {
		caPEM, readErr := os.ReadFile(tlsCfg.CAFile)
		if readErr != nil {
			return nil, fmt.Errorf("read tls ca file: %w", readErr)
		}

		if ok := roots.AppendCertsFromPEM(caPEM); !ok {
			return nil, errTLSCAFileHasNoCerts
		}
	}

	result := &tls.Config{
		RootCAs:    roots,
		ServerName: tlsCfg.ServerName,
		MinVersion: tls.VersionTLS13,
	}

	if tlsCfg.CertFile != "" && tlsCfg.KeyFile != "" {
		cert, certErr := tls.LoadX509KeyPair(tlsCfg.CertFile, tlsCfg.KeyFile)
		if certErr != nil {
			return nil, fmt.Errorf("load tls client certificate: %w", certErr)
		}

		result.Certificates = []tls.Certificate{cert}
	}

	return result, nil
}

// isLoopbackTarget reports whether target host resolves to loopback.
func isLoopbackTarget(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}

	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}
