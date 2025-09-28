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
}

// newService creates a service backed by the provided repository.
func newService(ctx context.Context, repository repo.Repository) (*service, error) {
	s := &service{
		repo: repository,
		state: &domain.State{
			Timestamp: time.Now(),
			IsEnabled: false,
		},
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

	s.state = &domain.State{
		Timestamp: time.Now(),
		LastActor: actor.Clone(),
		IsEnabled: isEnabled,
	}

	if s.repo != nil {
		if err := s.repo.Save(ctx, s.state); err != nil {
			logger.Errorf(ctx, "Failed to persist alarm state: %v", err)

			return nil, fmt.Errorf("persist state: %w", err)
		}
	}

	logger.InfoKV(ctx, "Alarm state updated", "is_enabled", s.state.IsEnabled, "actor", s.state.LastActor)

	result := s.state.Clone()

	return result, nil
}

// GetAlarmState returns the current alarm status.
func (s *service) GetAlarmState(ctx context.Context) *domain.State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logger.Info(ctx, "Alarm state requested", "is_enabled", s.state.IsEnabled, "actor", s.state.LastActor)

	result := s.state.Clone()

	return result
}
