//nolint:revive,nolintlint // Package name "common" is intentional for shared helpers.
package common

import (
	"fmt"
	"os"
	"os/user"

	pb "github.com/oshokin/alarm-button/internal/pb/v1"
)

// DetectActor gathers host and user information for audit trail.
// Returns a protobuf type because callers pass it directly to gRPC clients.
func DetectActor() (*pb.SystemActor, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("hostname: %w", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("current user: %w", err)
	}

	return &pb.SystemActor{
		Hostname: hostname,
		Username: currentUser.Username,
	}, nil
}
