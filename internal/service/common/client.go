//nolint:revive,nolintlint // Package name "common" is intentional for shared helpers.
package common

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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

// Option configures client behaviour.
type Option func(*Client)

// WithCallTimeout sets a default timeout for service calls.
func WithCallTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.callTimeout = timeout
		}
	}
}

var (
	// errAddressRequired is returned when a required address value is missing.
	errAddressRequired = errors.New("address must be provided")
	// errActorRequired is returned when an actor is not provided but is required for the operation.
	errActorRequired = errors.New("actor must be provided")
)

// Dial establishes a gRPC connection to the alarm server.
// Note: this uses insecure transport credentials; deploy on a trusted network
// or terminate TLS in a proxy until native TLS is added.
func Dial(_ context.Context, address string, opts ...Option) (*Client, error) {
	if address == "" {
		return nil, errAddressRequired
	}

	// Use the non-context NewClient API recommended by grpc-go
	// (DialContext is deprecated as of grpc-go v1.60+).
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
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

// callContext returns a context with the client's call timeout if configured,
// otherwise a cancellable child context without a deadline.
func (c *Client) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.callTimeout <= 0 {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, c.callTimeout)
}
