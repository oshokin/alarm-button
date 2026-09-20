package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/oshokin/alarm-button/internal/config"
	domain "github.com/oshokin/alarm-button/internal/domain/alarm"
	"github.com/oshokin/alarm-button/internal/fsutil"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
)

// Repository defines persistence operations for the alarm state.
type Repository interface {
	Load(ctx context.Context) (*domain.State, error)
	Save(ctx context.Context, state *domain.State) error
}

// FileRepository persists the alarm state to a JSON file on disk.
// JSON is produced and consumed via protobuf JSON (protojson) to stay
// compatible with the generated API types.
type FileRepository struct {
	// path is the filesystem location of the JSON state file.
	path string
	// mu protects concurrent access to the state file.
	mu sync.Mutex
}

// ErrNotFound is returned when the state file does not exist yet.
var ErrNotFound = errors.New("state not found")

// NewFileRepository creates a repository that reads/writes JSON at the provided path.
func NewFileRepository(path string) *FileRepository {
	return &FileRepository{
		path: filepath.Clean(path),
	}
}

// Load reads the state from disk.
func (r *FileRepository) Load(ctx context.Context) (*domain.State, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	contents, err := os.ReadFile(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("read state file: %w", err)
	}

	var protoState pb.AlarmStateResponse
	if err = protojson.Unmarshal(contents, &protoState); err != nil {
		return nil, fmt.Errorf("decode state file: %w", err)
	}

	return r.fromProto(&protoState), nil
}

// Save writes the state to disk using JSON representation.
func (r *FileRepository) Save(ctx context.Context, state *domain.State) error {
	ctxErr := ctx.Err()
	if ctxErr != nil {
		return ctxErr
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ctxErr = ctx.Err()
	if ctxErr != nil {
		return ctxErr
	}

	var (
		protoState     = r.toProto(state)
		marshalOptions = protojson.MarshalOptions{
			EmitUnpopulated: true,
		}
	)

	data, err := marshalOptions.Marshal(protoState)
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}

	ctxErr = ctx.Err()
	if ctxErr != nil {
		return ctxErr
	}

	if err = fsutil.WriteFileAtomic(r.path, data, config.DefaultFilePermissions); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

// fromProto converts protobuf AlarmStateResponse into the domain State model.
func (r *FileRepository) fromProto(protoState *pb.AlarmStateResponse) *domain.State {
	var (
		timestamp time.Time
		actor     *domain.Actor
	)

	if ts := protoState.GetTimestamp(); ts != nil {
		timestamp = ts.AsTime()
	}

	if protoActor := protoState.GetLastActor(); protoActor != nil {
		actor = &domain.Actor{
			Hostname: protoActor.GetHostname(),
			Username: protoActor.GetUsername(),
		}
	}

	return &domain.State{
		Timestamp: timestamp,
		LastActor: actor,
		IsEnabled: protoState.GetIsEnabled(),
	}
}

// toProto converts the domain State model into protobuf AlarmStateResponse.
func (r *FileRepository) toProto(state *domain.State) *pb.AlarmStateResponse {
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
