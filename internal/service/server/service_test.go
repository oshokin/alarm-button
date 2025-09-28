package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domain "github.com/oshokin/alarm-button/internal/domain/alarm"
	repo "github.com/oshokin/alarm-button/internal/repository/state"
)

var errTestLoad = errors.New("test load error")

// memoryRepository is a minimal in-memory Repository implementation for tests.
type memoryRepository struct {
	// state is the alarm state to return from Load operations.
	state *domain.State
	// loadErr is the error to return from Load operations.
	loadErr error
	// saved stores the last state passed to Save operations.
	saved *domain.State
}

// Load retrieves the current state from the memory repository.
// It returns a pointer to the domain.State and an error if the operation fails.
func (m *memoryRepository) Load(context.Context) (*domain.State, error) {
	return m.state, m.loadErr
}

// Save stores the provided domain.State in memory. It overwrites any previously saved state.
// This method always returns nil and does not perform any validation.
func (m *memoryRepository) Save(_ context.Context, s *domain.State) error {
	m.saved = s

	return nil
}

// TestNewService_LoadsStateOrDefaults asserts newService behavior on existing, missing, and error states.
func TestNewService_LoadsStateOrDefaults(t *testing.T) {
	t.Parallel()

	// Existing state.
	old := &domain.State{
		Timestamp: time.Unix(100, 0),
		LastActor: &domain.Actor{
			Hostname: "Oleg Shokin",
			Username: "o.shokin",
		},
		IsEnabled: true,
	}

	s, err := newService(context.Background(), &memoryRepository{state: old})

	require.NoError(t, err)
	require.Equal(t, old.IsEnabled, s.state.IsEnabled)
	require.Equal(t, old.LastActor, s.state.LastActor)

	// Not found -> default.
	s, err = newService(context.Background(), &memoryRepository{loadErr: repo.ErrNotFound})

	require.NoError(t, err)
	require.False(t, s.state.IsEnabled)

	// Other error.
	s, err = newService(context.Background(), &memoryRepository{loadErr: errTestLoad})

	require.Error(t, err)
	require.Nil(t, s)
}

// TestService_SetAndGet verifies SetAlarmState persists and GetAlarmState returns the latest state.
func TestService_SetAndGet(t *testing.T) {
	t.Parallel()

	repo := new(memoryRepository)
	s, err := newService(context.Background(), repo)
	require.NoError(t, err)

	actor := &domain.Actor{
		Hostname: "Oleg Shokin",
		Username: "o.shokin",
	}

	result, err := s.SetAlarmState(context.Background(), actor, true)

	require.NoError(t, err)
	require.True(t, result.IsEnabled)
	require.NotNil(t, result.LastActor)

	// Cloned.
	require.NotSame(t, actor, result.LastActor)
	require.NotNil(t, repo.saved)

	currentState := s.GetAlarmState(context.Background())
	require.True(t, currentState.IsEnabled)
}
