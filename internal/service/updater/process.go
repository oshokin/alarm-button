package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitchellh/go-ps"

	"github.com/oshokin/alarm-button/internal/proc"
)

// Process orchestration timing constants for version probe and shutdown waits.
const (
	versionCommandTimeout = 10 * time.Second
	stopProcessTimeout    = 10 * time.Second
	stopProcessPollDelay  = 100 * time.Millisecond
)

// Process ownership and lifecycle errors for updater role management.
var (
	errEmptyVersionOutput         = errors.New("empty version output")
	errPIDFileDoesNotMatchRole    = errors.New("pid file points to unexpected process")
	errMultipleMatchingProcesses  = errors.New("multiple running processes match executable name")
	errProcessTerminationTimedOut = errors.New("process termination timed out")
)

// stopProcessByPIDFile stops managed process by PID ownership when possible.
func (r *runner) stopProcessByPIDFile() error {
	pidFilePath := r.rolePIDFilePath()
	executablePath := r.roleExecutablePath()

	processID, err := proc.Read(pidFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return r.stopProcessByExecutableName(filepath.Base(executablePath))
		}

		return fmt.Errorf("read pid file: %w", err)
	}

	expectedName := filepath.Base(executablePath)

	return r.stopOwnedProcess(pidFilePath, processID, expectedName)
}

// stopOwnedProcess validates PID ownership and stops the owned process.
func (r *runner) stopOwnedProcess(path string, processID int, expectedName string) error {
	executableName, running, err := processExecutableName(processID)
	if err != nil {
		return err
	}

	if !running {
		return r.removePIDFileIfOwned(path, processID)
	}

	if executableName != expectedName {
		return fmt.Errorf(
			"%w: got %q, want %q",
			errPIDFileDoesNotMatchRole,
			executableName,
			expectedName,
		)
	}

	err = r.stopProcessByPID(processID)
	if err != nil {
		return err
	}

	return r.removePIDFileIfOwned(path, processID)
}

// removePIDFileIfOwned removes PID file only when it still points to processID.
func (r *runner) removePIDFileIfOwned(path string, processID int) error {
	err := proc.RemoveIfOwned(path, processID)
	if err != nil {
		return fmt.Errorf("remove pid file: %w", err)
	}

	return nil
}

// stopProcessByPID force-stops process and waits until it exits.
func (r *runner) stopProcessByPID(processID int) error {
	target, err := os.FindProcess(processID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", processID, err)
	}

	killErr := target.Kill()
	if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
		return fmt.Errorf("kill process %d: %w", processID, killErr)
	}

	deadline := time.Now().Add(stopProcessTimeout)

	for {
		running, runningErr := isProcessRunning(processID)
		if runningErr != nil {
			return runningErr
		}

		if !running {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %d", errProcessTerminationTimedOut, processID)
		}

		time.Sleep(stopProcessPollDelay)
	}
}

// processExecutableName resolves executable basename for PID if process exists.
func processExecutableName(pid int) (string, bool, error) {
	processes, err := ps.Processes()
	if err != nil {
		return "", false, fmt.Errorf("list processes: %w", err)
	}

	for _, process := range processes {
		if process.Pid() == pid {
			return process.Executable(), true, nil
		}
	}

	return "", false, nil
}

// isProcessRunning checks whether process with PID currently exists.
func isProcessRunning(pid int) (bool, error) {
	_, running, err := processExecutableName(pid)
	if err != nil {
		return false, err
	}

	return running, nil
}

// collectMatchingProcessIDs lists running processes matching executable basename.
func (r *runner) collectMatchingProcessIDs(executableName string) ([]int, error) {
	processes, err := ps.Processes()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}

	selfPID := os.Getpid()
	matching := make([]int, 0, 1)

	for _, process := range processes {
		if process.Pid() == selfPID {
			continue
		}

		if process.Executable() != executableName {
			continue
		}

		matching = append(matching, process.Pid())
	}

	return matching, nil
}

// stopProcessByExecutableName stops process by executable name when unambiguous.
func (r *runner) stopProcessByExecutableName(executableName string) error {
	matching, err := r.collectMatchingProcessIDs(executableName)
	if err != nil {
		return err
	}

	switch len(matching) {
	case 0:
		return nil
	case 1:
		return r.stopProcessByPID(matching[0])
	default:
		return fmt.Errorf("%w: %s", errMultipleMatchingProcesses, executableName)
	}
}

// startExecutable starts role executable with explicit config path.
func (r *runner) startExecutable() (*os.Process, error) {
	path := r.roleExecutablePath()
	configPath := r.configPath
	cmd := exec.CommandContext(context.Background(), path, "--config", configPath)

	cmd.Dir = filepath.Dir(path)
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return cmd.Process, nil
}

// stopStartedProcess terminates newly started process during rollback cleanup.
func (r *runner) stopStartedProcess(process *os.Process) error {
	if process == nil {
		return nil
	}

	killErr := process.Kill()
	if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
		return killErr
	}

	_, waitErr := process.Wait()
	if waitErr != nil && !errors.Is(waitErr, os.ErrProcessDone) {
		return waitErr
	}

	return nil
}

// detectLocalVersion executes "<binary> version --short" and parses semantic version.
func (r *runner) detectLocalVersion(ctx context.Context) (string, error) {
	executable := r.roleExecutablePath()

	cmdCtx, cancel := context.WithTimeout(ctx, versionCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, executable, "version", "--short")

	output, err := cmd.Output()
	if err != nil {
		exitErr, ok := errors.AsType[*exec.ExitError](err)
		if ok && exitErr != nil {
			return "", fmt.Errorf("detect version with %s: %w", executable, err)
		}

		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}

		return "", fmt.Errorf("detect version with %s: %w", executable, err)
	}

	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", fmt.Errorf("%w from %s", errEmptyVersionOutput, executable)
	}

	return version, nil
}
