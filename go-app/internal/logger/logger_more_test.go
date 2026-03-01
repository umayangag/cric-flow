package logger_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	logger "github.com/umayangag/cric-flow/go-app/internal/logger"
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

func TestSetupFromEnv_LevelDebug(t *testing.T) {
	require.NoError(t, os.Setenv("LOG_LEVEL", "debug"))
	require.NoError(t, os.Setenv("LOG_FORMAT", "json"))
	t.Cleanup(func() { _ = os.Unsetenv("LOG_LEVEL"); _ = os.Unsetenv("LOG_FORMAT") })
	out := captureStdout(func() {
		logger.SetupFromEnv()
		slog.Debug("debug-visible")
	})
	require.Contains(t, out, "debug-visible")
}

func TestSetupFromEnv_LevelError(t *testing.T) {
	require.NoError(t, os.Setenv("LOG_LEVEL", "error"))
	require.NoError(t, os.Setenv("LOG_FORMAT", "text"))
	t.Cleanup(func() { _ = os.Unsetenv("LOG_LEVEL"); _ = os.Unsetenv("LOG_FORMAT") })
	out := captureStdout(func() {
		logger.SetupFromEnv()
		slog.Info("info-hidden")
		slog.Error("error-visible")
	})
	require.NotContains(t, out, "info-hidden")
	require.Contains(t, out, "error-visible")
}

func TestSetupFromEnv_LevelWarning(t *testing.T) {
	require.NoError(t, os.Setenv("LOG_LEVEL", "warning"))
	require.NoError(t, os.Setenv("LOG_FORMAT", "text"))
	t.Cleanup(func() { _ = os.Unsetenv("LOG_LEVEL"); _ = os.Unsetenv("LOG_FORMAT") })
	out := captureStdout(func() {
		logger.SetupFromEnv()
		slog.Info("info-hidden")
		slog.Warn("warn-visible")
	})
	require.NotContains(t, out, "info-hidden")
	require.Contains(t, out, "warn-visible")
}

func TestL_ReturnsDefaultLogger(t *testing.T) {
	logger.SetupFromEnv()
	lg := logger.L()
	require.NotNil(t, lg)
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

func TestSetupFromEnv_LOG_COLOR_AddsANSI(t *testing.T) {
	require.NoError(t, os.Setenv("LOG_FORMAT", "text"))
	require.NoError(t, os.Setenv("LOG_LEVEL", "info"))
	require.NoError(t, os.Setenv("LOG_COLOR", "1"))
	t.Cleanup(func() {
		_ = os.Unsetenv("LOG_FORMAT")
		_ = os.Unsetenv("LOG_LEVEL")
		_ = os.Unsetenv("LOG_COLOR")
	})

	out := captureStdout(func() {
		logger.SetupFromEnv()
		slog.Error("err-red")
		slog.Warn("warn-yellow")
		slog.Info("info-plain")
	})
	require.Contains(t, out, "\033[31m") // ANSI red for error
	require.Contains(t, out, "\033[33m") // ANSI yellow for warn
	require.Contains(t, out, "\033[0m")  // ANSI reset
	require.Contains(t, out, "err-red")
	require.Contains(t, out, "warn-yellow")
	require.Contains(t, out, "info-plain")
}
