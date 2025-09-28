package alarm

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	domain "github.com/oshokin/alarm-button/internal/domain/alarm"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
)

// fakeService implements the alarm Service interface for unit testing the transport.
type fakeService struct {
	// setFn is a function type that sets the enabled state for a given actor.
	setFn func(ctx context.Context, actor *domain.Actor, isEnabled bool) (*domain.State, error)

	// state holds the current alarm state managed by the fake service.
	state *domain.State
}

// SetAlarmState sets the alarm state to enabled or disabled for the given actor.
// If a custom set function (setFn) is provided, it delegates the operation to it.
// Otherwise, it updates the internal state with the current timestamp, actor, and isEnabled status.
// Returns the updated state and an error, if any.
func (f *fakeService) SetAlarmState(ctx context.Context, actor *domain.Actor, isEnabled bool) (*domain.State, error) {
	if f.setFn != nil {
		return f.setFn(ctx, actor, isEnabled)
	}

	f.state = &domain.State{
		Timestamp: time.Now(),
		LastActor: actor,
		IsEnabled: isEnabled,
	}

	return f.state, nil
}

// GetAlarmState returns the current alarm state stored in the fake service.
func (f *fakeService) GetAlarmState(context.Context) *domain.State { return f.state }

// TestServer_SetAlarmState_Validation ensures invalid requests return InvalidArgument errors.
func TestServer_SetAlarmState_Validation(t *testing.T) {
	t.Parallel()

	s := NewServer(new(fakeService))

	_, err := s.SetAlarmState(context.Background(), nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	request := &pb.SetAlarmStateRequest{Actor: nil}

	_, err = s.SetAlarmState(context.Background(), request)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

// TestServer_Roundtrip exercises SetAlarmState and GetAlarmState end-to-end on the server implementation.
func TestServer_Roundtrip(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// Create server with fake service for isolated testing.
		s := NewServer(new(fakeService))

		// Create test request with actor information.
		request := &pb.SetAlarmStateRequest{
			Actor: &pb.SystemActor{
				Hostname: "test-hostname",
				Username: "test-user",
			},
			IsEnabled: true,
		}

		// Set alarm state and verify no error.
		_, err := s.SetAlarmState(context.Background(), request)
		require.NoError(t, err)

		// Wait for all async operations to complete.
		synctest.Wait()

		// Get alarm state and verify it was persisted correctly.
		response, err := s.GetAlarmState(context.Background(), new(pb.GetAlarmStateRequest))

		require.NoError(t, err)
		require.True(t, response.GetIsEnabled())
		require.NotNil(t, response.GetLastActor())
		require.Equal(t, "test-hostname", response.GetLastActor().GetHostname())
		require.Equal(t, "test-user", response.GetLastActor().GetUsername())
	})
}
