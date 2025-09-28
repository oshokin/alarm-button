package alarm

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	domain "github.com/oshokin/alarm-button/internal/domain/alarm"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
)

// Service abstracts the business operations the transport layer depends on.
type Service interface {
	SetAlarmState(ctx context.Context, actor *domain.Actor, isEnabled bool) (*domain.State, error)
	GetAlarmState(ctx context.Context) *domain.State
}

// Server implements the AlarmService gRPC API.
type Server struct {
	pb.UnimplementedAlarmServiceServer

	// service provides the business logic for alarm operations.
	service Service
}

// NewServer wires the provided service implementation into a gRPC handler.
func NewServer(service Service) *Server {
	return &Server{
		service: service,
	}
}

// SetAlarmState updates the alarm status and persists the new state.
func (s *Server) SetAlarmState(ctx context.Context, req *pb.SetAlarmStateRequest) (*pb.AlarmStateResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	if req.GetActor() == nil {
		return nil, status.Error(codes.InvalidArgument, "actor is required")
	}

	actor := toDomainActor(req.GetActor())

	state, err := s.service.SetAlarmState(ctx, actor, req.GetIsEnabled())
	if err != nil {
		return nil, status.Error(codes.Internal, "unable to persist state")
	}

	return toProtoState(state), nil
}

// GetAlarmState returns the current alarm status.
func (s *Server) GetAlarmState(ctx context.Context, _ *pb.GetAlarmStateRequest) (*pb.AlarmStateResponse, error) {
	state := s.service.GetAlarmState(ctx)

	return toProtoState(state), nil
}

// toDomainActor converts a protobuf SystemActor to a domain Actor.
func toDomainActor(actor *pb.SystemActor) *domain.Actor {
	if actor == nil {
		return nil
	}

	return &domain.Actor{
		Hostname: actor.GetHostname(),
		Username: actor.GetUsername(),
	}
}

// toProtoState converts a domain.State object to a pb.AlarmStateResponse protobuf message.
func toProtoState(state *domain.State) *pb.AlarmStateResponse {
	if state == nil {
		return &pb.AlarmStateResponse{}
	}

	var timestamp *timestamppb.Timestamp
	if !state.Timestamp.IsZero() {
		timestamp = timestamppb.New(state.Timestamp)
	}

	var actor *pb.SystemActor
	if state.LastActor != nil {
		actor = &pb.SystemActor{
			Hostname: state.LastActor.Hostname,
			Username: state.LastActor.Username,
		}
	}

	return &pb.AlarmStateResponse{
		Timestamp: timestamp,
		LastActor: actor,
		IsEnabled: state.IsEnabled,
	}
}
