package db

import (
	"os"
	"testing"
)

func TestBuildDSN(t *testing.T) {
	got := BuildDSN("u", "p", "h", "5432", "d", "disable")
	want := "postgres://u:p@h:5432/d?sslmode=disable"
	if got != want {
		t.Fatalf("dsn mismatch: got %q want %q", got, want)
	}
}

func TestGetenv_DefaultAndOverride(t *testing.T) {
	// When env is not set, returns default
	if v := getenv("NON_EXISTENT_ENV_XYZ", "def"); v != "def" {
		t.Fatalf("expected default, got %q", v)
	}
	// When env is set, returns env value
	t.Setenv("FOO_BAR", "hello")
	if v := getenv("FOO_BAR", "def"); v != "hello" {
		t.Fatalf("expected env override, got %q", v)
	}
	// Ensure empty string falls back to default
	os.Unsetenv("BAZ_QUX")
	if v := getenv("BAZ_QUX", "zzz"); v != "zzz" {
		t.Fatalf("expected default on empty, got %q", v)
	}
}
