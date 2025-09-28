// Package version exposes build metadata for the project.
//
// Variables Version, Commit, and BuildTime are injected at build time via
// Go ldflags and default to sensible values for local builds.
// Helper functions Short and Full render the version string for CLI output and logs.
package version
