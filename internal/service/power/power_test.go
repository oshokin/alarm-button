package power

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestShutdownCommand verifies OS-specific shutdown command mapping.
func TestShutdownCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		goos      string
		wantName  string
		wantArgs  []string
		expectErr bool
	}{
		{
			name:     "linux",
			goos:     "linux",
			wantName: "shutdown",
			wantArgs: []string{"-h", "now"},
		},
		{
			name:     "darwin",
			goos:     "darwin",
			wantName: "shutdown",
			wantArgs: []string{"-h", "now"},
		},
		{
			name:     "windows",
			goos:     "windows",
			wantName: "shutdown.exe",
			wantArgs: []string{"-s", "-f", "-t", "0"},
		},
		{
			name:      "unsupported",
			goos:      "plan9",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotName, gotArgs, err := shutdownCommand(tt.goos)
			if tt.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantName, gotName)
			require.Equal(t, tt.wantArgs, gotArgs)
		})
	}
}
