// Package proc provides atomic PID file helpers.
package proc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oshokin/alarm-button/internal/fsutil"
)

// fileMode is default permission used for PID files.
const fileMode = 0o600

// PID file helper errors.
var (
	errExecutablePathRequired = errors.New("executable path is required")
	errPIDMustBePositive      = errors.New("pid must be positive")
	errPIDFileEmpty           = errors.New("pid file is empty")
	errPIDFileInvalid         = errors.New("pid file has invalid value")
)

// DefaultPath returns a PID file path next to executable path.
func DefaultPath(executablePath string) (string, error) {
	if executablePath == "" {
		return "", errExecutablePathRequired
	}

	cleanPath := filepath.Clean(executablePath)
	baseName := filepath.Base(cleanPath)
	nameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))

	return filepath.Join(filepath.Dir(cleanPath), nameWithoutExt+".pid"), nil
}

// Write stores process PID in file atomically.
func Write(path string, pid int) error {
	if pid <= 0 {
		return errPIDMustBePositive
	}

	cleanPath := filepath.Clean(path)

	payload := []byte(strconv.Itoa(pid) + "\n")
	if err := fsutil.WriteFileAtomic(cleanPath, payload, fileMode); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}

	return nil
}

// Read loads process PID from file.
func Read(path string) (int, error) {
	cleanPath := filepath.Clean(path)

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return 0, err
	}

	value := strings.TrimSpace(string(content))
	if value == "" {
		return 0, errPIDFileEmpty
	}

	pid, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", errPIDFileInvalid, value)
	}

	if pid <= 0 {
		return 0, fmt.Errorf("%w: %d", errPIDFileInvalid, pid)
	}

	return pid, nil
}

// RemoveIfOwned removes file when it points to provided PID.
func RemoveIfOwned(path string, pid int) error {
	currentPID, err := Read(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	if currentPID != pid {
		return nil
	}

	removeErr := os.Remove(filepath.Clean(path))
	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return fmt.Errorf("remove pid file: %w", removeErr)
	}

	return nil
}
