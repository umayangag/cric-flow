package logger_test

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	logger "github.com/umayangag/cric-flow/go-app/internal/logger"
)

// captureStdout captures stdout while fn runs and returns it as a string.
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}

func unsetEnv(keys ...string) {
	for _, k := range keys {
		_ = os.Unsetenv(k)
	}
}

// Not parallel: tests mutate process env and global slog default handler.
func TestSetupFromEnv_TextFormat_LevelFiltering(t *testing.T) {
	// Arrange
	unsetEnv("LOG_FORMAT", "LOG_LEVEL")
	require.NoError(t, os.Setenv("LOG_FORMAT", "text"))
	require.NoError(t, os.Setenv("LOG_LEVEL", "error"))

	// Act
	out := captureStdout(func() {
		logger.SetupFromEnv() // sets slog default
		slog.Warn("warn should be hidden")
		slog.Error("err should be visible")
	})

	// Assert
	require.Falsef(t, strings.Contains(out, "warn should be hidden") || strings.Contains(out, "level=WARN"),
		"expected WARN to be filtered out at error level, got: %s", out)
	require.Truef(t, strings.Contains(out, "err should be visible") && strings.Contains(out, "level=ERROR"),
		"expected ERROR to be present in text output, got: %s", out)
}

func TestSetupFromEnv_JSONFormat_Basic(t *testing.T) {
	// Arrange
	unsetEnv("LOG_FORMAT", "LOG_LEVEL")
	require.NoError(t, os.Setenv("LOG_FORMAT", "json"))
	require.NoError(t, os.Setenv("LOG_LEVEL", "info"))

	// Act
	out := captureStdout(func() {
		logger.SetupFromEnv()
		slog.Info("hello", "k", 1)
	})

	// Assert (JSON-like content)
	require.Contains(t, out, "\"msg\":\"hello\"")
	require.Contains(t, out, "\"level\"")
}
