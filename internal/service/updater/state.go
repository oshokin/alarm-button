package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/fsutil"
)

// loadUpdateState loads updater state from disk with defaults when file is absent.
func loadUpdateState(path string) (*UpdateState, error) {
	contents, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &UpdateState{}, nil
		}

		return nil, fmt.Errorf("read update state: %w", err)
	}

	var state UpdateState
	if err = json.Unmarshal(contents, &state); err != nil {
		return nil, fmt.Errorf("decode update state: %w", err)
	}

	return &state, nil
}

// saveUpdateState atomically persists updater state JSON to disk.
func (r *runner) saveUpdateState(state *UpdateState) error {
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode update state: %w", err)
	}

	err = fsutil.WriteFileAtomic(r.statePath, append(payload, '\n'), config.DefaultFilePermissions)
	if err != nil {
		return fmt.Errorf("write update state: %w", err)
	}

	return nil
}
