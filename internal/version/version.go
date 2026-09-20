package version

import "fmt"

// Build metadata values overridden via linker flags at build time.
var (
	// Version is the semantic version of the build. It can be overridden via ldflags.
	Version = "dev"
	// Commit is the short git SHA embedded at build time (or "none").
	Commit = "none"
)

// Short returns only the semantic version string.
func Short() string {
	return Version
}

// Full returns a human-readable version string with commit information.
func Full() string {
	return fmt.Sprintf("version: %s, commit: %s", Version, Commit)
}
