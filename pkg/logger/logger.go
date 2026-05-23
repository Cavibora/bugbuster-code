// Package logger provides structured logging for BugBuster Code.
// It wraps Go's standard log/slog with convenience methods.
package logger

import (
	"context"
	"log/slog"
	"os"
)

// Level — log level
type Level = slog.Level

const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// L — global logger
var L *slog.Logger

func init() {
	L = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// Init initializes logger
func Init(level string, jsonFormat bool, filePath string) error {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	var writer = os.Stderr
	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		writer = f
	}

	opts := &slog.HandlerOptions{Level: lvl}
	if jsonFormat {
		L = slog.New(slog.NewJSONHandler(writer, opts))
	} else {
		L = slog.New(slog.NewTextHandler(writer, opts))
	}

	return nil
}

// SetLevel changes log level
func SetLevel(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return
	}
	L = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

// Convenience functions

// Debug logs a message at debug level using the global logger.
func Debug(msg string, args ...any) { L.Debug(msg, args...) }

// Info logs a message at info level using the global logger.
func Info(msg string, args ...any)  { L.Info(msg, args...) }

// Warn logs a message at warning level using the global logger.
func Warn(msg string, args ...any)  { L.Warn(msg, args...) }

// Error logs a message at error level using the global logger.
func Error(msg string, args ...any) { L.Error(msg, args...) }

// With returns child logger with context fields
func With(args ...any) *slog.Logger {
	return L.With(args...)
}

// IsDebug returns true if log level is debug
func IsDebug() bool {
	return L.Enabled(context.Background(), slog.LevelDebug)
}
