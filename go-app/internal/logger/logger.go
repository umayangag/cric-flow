package logger

import (
	"log/slog"
	"os"
	"strings"
)

// SetupFromEnv configures the global slog default logger.
//
// Environment variables:
//
//	LOG_FORMAT = json|text (default: text)
//	LOG_LEVEL  = debug|info|warn|error (default: info)
func SetupFromEnv() *slog.Logger {
	level := new(slog.LevelVar)
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn", "warning":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT"))) {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	default:
		// default to JSON for clearer structured logs
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	lg := slog.New(handler)
	slog.SetDefault(lg)
	return lg
}

// L returns the global default slog logger (after SetupFromEnv).
func L() *slog.Logger { return slog.Default() }
