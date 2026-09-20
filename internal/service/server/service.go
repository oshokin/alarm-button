package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	domain "github.com/oshokin/alarm-button/internal/domain/alarm"
	"github.com/oshokin/alarm-button/internal/logger"
	repo "github.com/oshokin/alarm-button/internal/repository/state"
)

// service encapsulates the alarm business logic and persistence orchestration.
// It is unexported to keep the transport decoupled from the implementation.
type service struct {
	// repo handles persistent storage of alarm state.
	repo repo.Repository
	// state is the current in-memory alarm state.
	state *domain.State
	// mu protects concurrent access to the alarm state.
	mu sync.RWMutex
	// now is injected for deterministic tests.
	now func() time.Time
}

// newService creates a service backed by the provided repository.
func newService(ctx context.Context, repository repo.Repository) (*service, error) {
	s := &service{
		repo: repository,
		state: &domain.State{
			Timestamp: time.Now(),
			IsEnabled: false,
		},
		now: time.Now,
	}

	if repository == nil {
		return s, nil
	}

	state, err := repository.Load(ctx)
	switch {
	case err == nil:
		if state != nil {
			s.state = state
		}
	case errors.Is(err, repo.ErrNotFound):
		// Keep default state.
	default:
		return nil, fmt.Errorf("load state: %w", err)
	}

	return s, nil
}

// SetAlarmState updates the alarm status and persists the new state.
func (s *service) SetAlarmState(ctx context.Context, actor *domain.Actor, isEnabled bool) (*domain.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := &domain.State{
		Timestamp: s.now(),
		LastActor: actor.Clone(),
		IsEnabled: isEnabled,
	}

	if s.repo != nil {
		if err := s.repo.Save(ctx, next); err != nil {
			logger.ErrorKV(ctx, "Failed to persist alarm state", "error", err)
			return nil, fmt.Errorf("persist state: %w", err)
		}
	}

	s.state = next
	logger.InfoKV(
		ctx,
		"Alarm state updated",
		"is_enabled",
		next.IsEnabled,
		"actor",
		next.LastActor.String(),
		"timestamp",
		next.Timestamp.UTC().Format(time.RFC3339Nano),
	)

	result := next.Clone()

	return result, nil
}

// GetAlarmState returns the current alarm status.
func (s *service) GetAlarmState(ctx context.Context) *domain.State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logger.InfoKV(
		ctx,
		"Alarm state requested",
		"is_enabled",
		s.state.IsEnabled,
		"actor",
		s.state.LastActor.String(),
		"timestamp",
		s.state.Timestamp.UTC().Format(time.RFC3339Nano),
	)

	result := s.state.Clone()

	return result
}
