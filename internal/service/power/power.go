package power

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// windowsShutdownTimeout is the delay in seconds for Windows shutdown command.
const windowsShutdownTimeout = "0"

// ErrUnsupportedOS indicates the current OS is not supported for shutdown.
var ErrUnsupportedOS = errors.New("unsupported operating system")

// Shutdown triggers an OS shutdown command using common, built-in tools:
// - Linux/macOS: `shutdown -h now`
// - Windows:     `shutdown.exe -s -f -t 0` (force, no delay)
// The commands are started asynchronously; the OS takes over the rest.
func Shutdown(ctx context.Context) error {
	osName := strings.ToLower(runtime.GOOS)

	switch {
	case strings.Contains(osName, "linux") || strings.Contains(osName, "darwin"):
		return exec.CommandContext(ctx, "shutdown", "-h", "now").Start()
	case strings.Contains(osName, "windows"):
		return exec.CommandContext(ctx, "shutdown.exe", "-s", "-f", "-t", windowsShutdownTimeout).Start()
	default:
		return fmt.Errorf("unsupported operating system: %s: %w", runtime.GOOS, ErrUnsupportedOS)
	}
}
