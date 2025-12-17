package logger_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	logger "github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
)

// Not parallel: mutates env and global slog handler.
func TestSetupFromEnv_Defaults_InfoJSON(t *testing.T) {
	require.NoError(t, os.Unsetenv("LOG_FORMAT"))
	require.NoError(t, os.Unsetenv("LOG_LEVEL"))

	out := captureStdout(func() {
		logger.SetupFromEnv()
		slog.Info("hello-default")
		slog.Debug("debug-hidden")
	})
	require.Contains(t, out, "\"msg\":\"hello-default\"")
	require.NotContains(t, out, "debug-hidden")
}

func TestSetupFromEnv_InvalidValues_FallBack(t *testing.T) {
	require.NoError(t, os.Setenv("LOG_FORMAT", "invalid"))
	require.NoError(t, os.Setenv("LOG_LEVEL", "nope"))

	out := captureStdout(func() {
		logger.SetupFromEnv()
		slog.Info("info-visible")
		slog.Debug("debug-hidden")
	})
	require.Contains(t, out, "info-visible")
	require.NotContains(t, out, "debug-hidden")
}

func TestSetupFromEnv_Idempotent(t *testing.T) {
	require.NoError(t, os.Setenv("LOG_FORMAT", "text"))
	require.NoError(t, os.Setenv("LOG_LEVEL", "warn"))

	// Call twice, ensure no panic and WARN passes while INFO is filtered
	out := captureStdout(func() {
		logger.SetupFromEnv()
		logger.SetupFromEnv()
		slog.Info("info-hidden")
		slog.Warn("warn-visible")
	})
	require.NotContains(t, out, "info-hidden")
	require.Contains(t, out, "warn-visible")
}
