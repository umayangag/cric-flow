package logger

import (
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestSetupFromEnv_Defaults_InfoJSON(t *testing.T) {
	_ = os.Unsetenv("LOG_FORMAT")
	_ = os.Unsetenv("LOG_LEVEL")

	out := captureStdout(func() {
		SetupFromEnv()
		slog.Info("hello-default")
		slog.Debug("debug-hidden")
	})
	if !strings.Contains(out, "\"msg\":\"hello-default\"") {
		t.Fatalf("expected info message in default JSON logs, got: %s", out)
	}
	if strings.Contains(out, "debug-hidden") {
		t.Fatalf("did not expect debug to appear at default info level, got: %s", out)
	}
}

func TestSetupFromEnv_InvalidValues_FallBack(t *testing.T) {
	_ = os.Setenv("LOG_FORMAT", "invalid")
	_ = os.Setenv("LOG_LEVEL", "nope")

	out := captureStdout(func() {
		SetupFromEnv()
		slog.Info("info-visible")
		slog.Debug("debug-hidden")
	})
	if !strings.Contains(out, "info-visible") {
		t.Fatalf("expected info to be visible with invalid level fallback, got: %s", out)
	}
	if strings.Contains(out, "debug-hidden") {
		t.Fatalf("expected debug to be hidden with invalid level fallback, got: %s", out)
	}
}

func TestSetupFromEnv_Idempotent(t *testing.T) {
	_ = os.Setenv("LOG_FORMAT", "text")
	_ = os.Setenv("LOG_LEVEL", "warn")

	// Call twice, ensure no panic and WARN passes while INFO is filtered
	out := captureStdout(func() {
		SetupFromEnv()
		SetupFromEnv()
		slog.Info("info-hidden")
		slog.Warn("warn-visible")
	})
	if strings.Contains(out, "info-hidden") {
		t.Fatalf("did not expect info at warn level, got: %s", out)
	}
	if !strings.Contains(out, "warn-visible") {
		t.Fatalf("expected warn to be visible at warn level, got: %s", out)
	}
}
