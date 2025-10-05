package updater

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mitchellh/go-ps"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/logger"
	"github.com/oshokin/alarm-button/internal/version"

	// Ensure SHA512 available for checksum calculation.
	_ "crypto/sha512"
)

var errHashUnavailable = errors.New("hash function unavailable")

const (
	// VersionFilename stores the update description pushed to clients.
	VersionFilename = "alarm-button-version.yaml"

	// MarkerFilename marks that the updater is running right now to avoid parallel execution.
	MarkerFilename = "alarm-button-update-marker.bin"

	// DefaultFileMode is used when producing artifacts for distribution.
	DefaultFileMode os.FileMode = 0o755

	// DefaultChecksumFunction is used to calculate update file hashes.
	DefaultChecksumFunction crypto.Hash = crypto.SHA512

	// Base executable names; platform helpers append extension when needed.
	baseServerExecutable  = "alarm-server"
	baseCheckerExecutable = "alarm-checker"
	baseUpdaterExecutable = "alarm-updater"

	// markerLifetime is the period after which a stale update marker is ignored.
	markerLifetime = 30 * time.Second

	// defaultMapCapacity is the default initial capacity for maps and slices.
	defaultMapCapacity = 16

	// versionCommandTimeout is the timeout for executing version commands.
	versionCommandTimeout = 10 * time.Second
)

// AllowedUserRoles returns artifact lists per role for the current platform.
func AllowedUserRoles() map[string][]string {
	return map[string][]string{
		"client": {
			"alarm-button-on" + getExecutableExtension(),
			checkerExecutable(),
			updaterExecutable(),
			config.DefaultConfigFilename,
		},
		"server": {
			"alarm-button-off" + getExecutableExtension(),
			serverExecutable(),
			updaterExecutable(),
			config.DefaultConfigFilename,
		},
	}
}

// ExecutablesByUserRoles returns the restart targets per role for the current platform.
func ExecutablesByUserRoles() map[string]string {
	return map[string]string{
		"client": checkerExecutable(),
		"server": serverExecutable(),
	}
}

// FilesWithChecksum returns the list of artifacts to hash for this platform.
func FilesWithChecksum() []string {
	return []string{
		"alarm-button-off" + getExecutableExtension(),
		"alarm-button-on" + getExecutableExtension(),
		checkerExecutable(),
		serverExecutable(),
		updaterExecutable(),
		config.DefaultConfigFilename,
	}
}

// Description contains metadata about a published release.
type Description struct {
	// VersionNumber is the semantic version of this release.
	VersionNumber string `yaml:"version"`
	// Files maps filenames to their base64-encoded checksums.
	Files map[string]string `yaml:"files"`
	// Roles maps role names to lists of files required for that role.
	Roles map[string][]string `yaml:"roles"`
	// Executables maps role names to their primary executable files.
	Executables map[string]string `yaml:"executables"`
}

// NewDescription produces a Description initialized with defaults.
func NewDescription() *Description {
	return &Description{
		VersionNumber: version.Short(),
		Files:         make(map[string]string, defaultMapCapacity),
		Roles:         make(map[string][]string, defaultMapCapacity),
		Executables:   make(map[string]string, defaultMapCapacity),
	}
}

// GetFileChecksum returns checksum bytes for a file using DefaultChecksumFunction.
func GetFileChecksum(path string) ([]byte, error) {
	contents, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}

	if !DefaultChecksumFunction.Available() {
		return nil, fmt.Errorf("checksum calculation not possible: %w", errHashUnavailable)
	}

	hasher := DefaultChecksumFunction.New()
	if _, err = hasher.Write(contents); err != nil {
		return nil, fmt.Errorf("calculate checksum: %w", err)
	}

	hash := hasher.Sum(nil)

	return hash, nil
}

// IsUpdaterRunningNow checks presence of a marker file and attempts recovery if it looks stale.
func IsUpdaterRunningNow(ctx context.Context) bool {
	logger.Info(ctx, "Checking for the presence of an update marker")

	fileInfo, err := os.Stat(MarkerFilename)
	if err == nil {
		if time.Since(fileInfo.ModTime()) <= markerLifetime {
			return true
		}

		logger.Info(ctx, "The update marker is too old, attempting cleanup")

		if err = terminateProcessByName(updaterExecutable()); err != nil {
			return true
		}

		if err = os.Remove(MarkerFilename); err != nil {
			return true
		}

		return false
	}

	if errors.Is(err, os.ErrNotExist) {
		logger.Info(ctx, "Update marker not found, continuing")
		return false
	}

	logger.Infof(ctx, "Unable to read update marker: %v", err)

	return false
}

// terminateProcessByName tries to kill processes with the provided executable name.
func terminateProcessByName(processName string) error {
	processList, err := ps.Processes()
	if err != nil {
		return err
	}

	thisProcessID := os.Getpid()

	for _, process := range processList {
		if process.Pid() == thisProcessID {
			continue
		}

		if process.Executable() != processName {
			continue
		}

		var runningProcess *os.Process

		runningProcess, err = os.FindProcess(process.Pid())
		if err != nil {
			return err
		}

		if err = runningProcess.Kill(); err != nil {
			return err
		}
	}

	return nil
}

// getExecutableExtension returns ".exe" on Windows and "" elsewhere.
func getExecutableExtension() string {
	if strings.Contains(strings.ToLower(runtime.GOOS), "windows") {
		return ".exe"
	}

	return ""
}

func serverExecutable() string {
	return baseServerExecutable + getExecutableExtension()
}

func checkerExecutable() string {
	return baseCheckerExecutable + getExecutableExtension()
}

func updaterExecutable() string {
	return baseUpdaterExecutable + getExecutableExtension()
}

// sliceToSet converts a slice to a set for quick lookups.
func sliceToSet[T comparable](elements []T) map[T]struct{} {
	result := make(map[T]struct{}, len(elements))
	for _, value := range elements {
		result[value] = struct{}{}
	}

	return result
}
