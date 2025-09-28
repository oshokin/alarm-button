// Package checker periodically polls the alarm server for the current state
// and, when the alarm is enabled, initiates a local OS shutdown unless the
// CLI is run in debug mode.
package checker
