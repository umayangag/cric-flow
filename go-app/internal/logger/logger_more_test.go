package logger_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/logger"
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

// GO-15: colorHandler.WithAttrs and WithGroup used to return the receiver unchanged, so
// any slog.With(...) or WithGroup(...) attrs vanished the moment LOG_COLOR=1 selected this
// handler as the default -- silently, since Handle still produced a well-formed line, just
// one with the caller's attrs missing from it. TestLogger_WithAttrsAndWithGroup below does
// not catch this: it never sets LOG_COLOR=1, and only asserts the log message text is
// present, not that the attrs survived. This sets LOG_COLOR=1 and asserts the attrs
// themselves -- including one added after WithGroup, which must nest inside that group --
// are actually in the line.
func TestSetupFromEnv_LOG_COLOR_KeepsWithAttrsAndWithGroup(t *testing.T) {
	require.NoError(t, os.Setenv("LOG_FORMAT", "json"))
	require.NoError(t, os.Setenv("LOG_LEVEL", "info"))
	require.NoError(t, os.Setenv("LOG_COLOR", "1"))
	t.Cleanup(func() {
		_ = os.Unsetenv("LOG_FORMAT")
		_ = os.Unsetenv("LOG_LEVEL")
		_ = os.Unsetenv("LOG_COLOR")
	})

	out := captureStdout(func() {
		logger.SetupFromEnv()
		slog.Default().With("request_id", "abc-123").Error("with-attrs-and-color")
		slog.Default().WithGroup("db").With("table", "match").Info("with-group-and-color")
	})

	require.Contains(t, out, `"request_id":"abc-123"`, "an attr added with .With must survive LOG_COLOR=1")
	require.Contains(t, out, `"db":{`, "a group added with .WithGroup must survive LOG_COLOR=1")
	require.Contains(t, out, `"table":"match"`, "an attr added after .WithGroup must nest inside that group")
}

// TestLogger_WithAttrsAndWithGroup exercises the handler's WithAttrs and WithGroup (slog calls them when using With/WithGroup).
func TestLogger_WithAttrsAndWithGroup(t *testing.T) {
	require.NoError(t, os.Setenv("LOG_FORMAT", "json"))
	require.NoError(t, os.Setenv("LOG_LEVEL", "info"))
	t.Cleanup(func() { _ = os.Unsetenv("LOG_FORMAT"); _ = os.Unsetenv("LOG_LEVEL") })

	out := captureStdout(func() {
		logger.SetupFromEnv()
		slog.Default().With("attr", "value").Info("with-attrs")
		slog.Default().WithGroup("group").Info("with-group")
	})
	require.Contains(t, out, "with-attrs")
	require.Contains(t, out, "with-group")
}
