package power

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
)

// ErrUnsupportedOS indicates the current OS is not supported for shutdown.
var ErrUnsupportedOS = errors.New("unsupported operating system")

// Shutdown triggers an OS shutdown command using common, built-in tools:
// - Linux/macOS: `shutdown -h now`
// - Windows:     `shutdown.exe -s -f -t 0` (force, no delay)
// The commands are started asynchronously; the OS takes over the rest.
func Shutdown(ctx context.Context) error {
	name, args, err := shutdownCommand(runtime.GOOS)
	if err != nil {
		return err
	}

	err = exec.CommandContext(ctx, name, args...).Start()
	if err != nil {
		return fmt.Errorf("start shutdown command: %w", err)
	}

	return nil
}

// shutdownCommand resolves shutdown executable and arguments for target OS.
func shutdownCommand(goos string) (name string, args []string, err error) {
	switch goos {
	case "linux", "darwin":
		return "shutdown", []string{"-h", "now"}, nil
	case "windows":
		return "shutdown.exe", []string{"-s", "-f", "-t", "0"}, nil
	default:
		return "", nil, fmt.Errorf("%s: %w", goos, ErrUnsupportedOS)
	}
}
