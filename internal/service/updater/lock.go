package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// updateLock represents acquired updater lock file handle by path.
type updateLock struct {
	// path is absolute lock file path.
	path string
}

// errUpdateAlreadyRunning indicates active updater lock cannot be acquired.
var errUpdateAlreadyRunning = errors.New("update is already running")

// acquireUpdateLock obtains exclusive updater lock with stale-lock recovery.
func acquireUpdateLock(path string) (*updateLock, error) {
	lockPath := filepath.Clean(path)

	if err := createUpdateLock(lockPath); err == nil {
		return &updateLock{path: lockPath}, nil
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}

	stale, err := isStaleLock(lockPath)
	if err != nil {
		return nil, err
	}

	if !stale {
		return nil, errUpdateAlreadyRunning
	}

	if err = os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale update lock: %w", err)
	}

	if err = createUpdateLock(lockPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, errUpdateAlreadyRunning
		}

		return nil, err
	}

	return &updateLock{path: lockPath}, nil
}

// createUpdateLock creates lock file and stores current updater PID in JSON.
func createUpdateLock(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return err
		}

		return fmt.Errorf("create update lock: %w", err)
	}

	defer func() { _ = file.Close() }()

	payload := struct {
		PID int `json:"pid"`
	}{
		PID: os.Getpid(),
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode update lock: %w", err)
	}

	_, err = file.Write(append(encoded, '\n'))
	if err != nil {
		return fmt.Errorf("write update lock: %w", err)
	}

	if err = file.Sync(); err != nil {
		return fmt.Errorf("sync update lock: %w", err)
	}

	return nil
}

// isStaleLock checks whether existing lock file belongs to dead process.
func isStaleLock(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}

		return false, fmt.Errorf("read update lock: %w", err)
	}

	processID, ok := decodeLockPID(content)
	if !ok {
		return true, nil
	}

	if processID <= 0 {
		return true, nil
	}

	running, err := isProcessRunning(processID)
	if err != nil {
		return false, err
	}

	return !running, nil
}

// decodeLockPID decodes process identifier payload from lock file.
func decodeLockPID(content []byte) (int, bool) {
	payload := struct {
		PID int `json:"pid"`
	}{}

	if err := json.Unmarshal(content, &payload); err != nil {
		return 0, false
	}

	return payload.PID, true
}

// Release releases acquired updater lock.
func (l *updateLock) Release() error {
	if l == nil {
		return nil
	}

	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove update lock: %w", err)
	}

	return nil
}
