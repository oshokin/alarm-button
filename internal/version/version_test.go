package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestVersionStrings ensures Short and Full return non-empty consistent information.
func TestVersionStrings(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, Short())
	require.Contains(t, Full(), Short())
}

// TestShort verifies that the Short function returns
// the correct version string for different version values.
func TestShort(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "default version",
			version:  "1.0.0",
			expected: "1.0.0",
		},
		{
			name:     "custom version",
			version:  "2.1.3",
			expected: "2.1.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			if got := Short(); got != tt.expected {
				t.Errorf("Short() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestFull verifies that the Full function returns the correct formatted string
// with version, commit hash, and build timestamp information.
func TestFull(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		commit    string
		buildTime string
		expected  string
	}{
		{
			name:      "default values",
			version:   "1.0.0",
			commit:    "none",
			buildTime: "unknown",
			expected:  "version: 1.0.0, commit: none, built at: unknown",
		},
		{
			name:      "custom values",
			version:   "2.1.3",
			commit:    "abc123",
			buildTime: "2024-01-15T10:30:00Z",
			expected:  "version: 2.1.3, commit: abc123, built at: 2024-01-15T10:30:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			Commit = tt.commit
			BuildTime = tt.buildTime

			if got := Full(); got != tt.expected {
				t.Errorf("Full() = %v, want %v", got, tt.expected)
			}
		})
	}
}
