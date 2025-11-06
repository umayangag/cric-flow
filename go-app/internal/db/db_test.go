package db

import (
	"os"
	"testing"
)

func TestBuildDSN(t *testing.T) {
	got := BuildDSN("user", "pass", "host", "5432", "dbname", "disable")
	want := "postgres://user:pass@host:5432/dbname?sslmode=disable"
	if got != want {
		t.Fatalf("BuildDSN mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestGetenv_DefaultWhenUnset(t *testing.T) {
	const key = "DB_TEST_UNSET"
	_ = os.Unsetenv(key)
	if v := getenv(key, "fallback"); v != "fallback" {
		t.Fatalf("getenv want fallback, got %q", v)
	}
}

func TestGetenv_ValueWhenSet(t *testing.T) {
	const key = "DB_TEST_SET"
	t.Setenv(key, "value")
	if v := getenv(key, "fallback"); v != "value" {
		t.Fatalf("getenv want value, got %q", v)
	}
}
