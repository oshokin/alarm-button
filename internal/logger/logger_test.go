package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

// TestParseLogLevel verifies mapping from strings to zapcore.Level and handling of unknown values.
func TestParseLogLevel(t *testing.T) {
	t.Parallel()

	cases := map[string]zapcore.Level{
		"debug": zapcore.DebugLevel,
		"info":  zapcore.InfoLevel,
		"warn":  zapcore.WarnLevel,
		"error": zapcore.ErrorLevel,
		"panic": zapcore.PanicLevel,
		"fatal": zapcore.FatalLevel,
	}
	for s, lvl := range cases {
		got, ok := ParseLogLevel(s)
		require.True(t, ok)
		require.Equal(t, lvl, got)
	}

	_, ok := ParseLogLevel("unknown")
	require.False(t, ok)
}
