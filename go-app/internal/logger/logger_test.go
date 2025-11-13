package logger

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
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

func TestSetupFromEnv_TextFormat_LevelFiltering(t *testing.T) {
	unsetEnv("LOG_FORMAT", "LOG_LEVEL")
	// Force text format and error level
	_ = os.Setenv("LOG_FORMAT", "text")
	_ = os.Setenv("LOG_LEVEL", "error")

	out := captureStdout(func() {
		SetupFromEnv() // sets slog default
		slog.Warn("warn should be hidden")
		slog.Error("err should be visible")
	})

	if strings.Contains(out, "warn should be hidden") || strings.Contains(out, "level=WARN") {
		t.Fatalf("expected WARN to be filtered out at error level, got output: %s", out)
	}
	if !strings.Contains(out, "err should be visible") || !strings.Contains(out, "level=ERROR") {
		t.Fatalf("expected ERROR to be present in text format, got: %s", out)
	}
}

func TestSetupFromEnv_JSONFormat_Basic(t *testing.T) {
	unsetEnv("LOG_FORMAT", "LOG_LEVEL")
	_ = os.Setenv("LOG_FORMAT", "json")
	_ = os.Setenv("LOG_LEVEL", "info")

	out := captureStdout(func() {
		SetupFromEnv()
		slog.Info("hello", "k", 1)
	})

	// Expect JSON-like output containing keys
	if !strings.Contains(out, "\"msg\":\"hello\"") {
		t.Fatalf("expected JSON to contain msg=hello, got: %s", out)
	}
	if !strings.Contains(out, "\"level\"") {
		t.Fatalf("expected JSON to contain level key, got: %s", out)
	}
}
