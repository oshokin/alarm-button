package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// coreWithLevel wraps a zapcore.Core with a specific log level.
type coreWithLevel struct {
	zapcore.Core

	// level is the minimum log level for this core to process messages.
	level zapcore.Level
}

// Enabled returns true if the provided log level is enabled for logging by the core.
// It calls the Enabled method on the wrapped zapcore.Level.
func (c *coreWithLevel) Enabled(l zapcore.Level) bool {
	return c.level.Enabled(l)
}

// Check adds the core to a checked entry if the log entry level is enabled for logging.
// It returns the checked entry with the added core or the original checked entry
// if the level is disabled.
//
//nolint:gocritic // AddCore requires ent to be passed by value.
func (c *coreWithLevel) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}

	return ce
}

// With returns a new core with added fields to the wrapped core.
// It returns a new coreWithLevel with the same level as the original core.
//
//nolint:ireturn,nolintlint // Returning zapcore.Core is intended for zap integration.
func (c *coreWithLevel) With(fields []zapcore.Field) zapcore.Core {
	return &coreWithLevel{
		c.Core.With(fields),
		c.level,
	}
}

// WithLevel is an option that creates a logger with the specified logging level based on an existing logger.
// It returns a zap.Option that wraps the existing core in a coreWithLevel with the specified level.
//
//nolint:ireturn,nolintlint // Returning zap.Option is intended for zap integration.
func WithLevel(lvl zapcore.Level) zap.Option {
	return zap.WrapCore(
		func(core zapcore.Core) zapcore.Core {
			return &coreWithLevel{core, lvl}
		})
}
