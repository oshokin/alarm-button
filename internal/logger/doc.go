// Package logger provides a small wrapper around zap to offer:
//   - a global sugared logger with a sane console encoder,
//   - context helpers (ToContext/FromContext/WithName/WithKV/WithFields),
//   - level configuration and parsing utilities,
//   - convenience functions (Infof, ErrorKV, etc.).
//
// All services accept a context and extract the logger from it, enabling
// scoped, structured logging throughout the codebase.
package logger
