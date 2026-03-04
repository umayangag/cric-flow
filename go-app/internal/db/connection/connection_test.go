package connection

import (
	"strings"
	"testing"
)

func TestBuildDSN_ComposesExpectedURL(t *testing.T) {
	got := BuildDSN("user", "x", "host", "5432", "dbname", "disable")
	wantSuffix := "://user:x@host:5432/dbname?sslmode=disable"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("BuildDSN() = %q, want suffix %q", got, wantSuffix)
	}
}

func TestGetenv_ReturnsEnvOrDefault(t *testing.T) {
	t.Setenv("SOME_KEY", "value")
	if got := getenv("SOME_KEY", "default"); got != "value" {
		t.Fatalf("getenv with env set = %q, want %q", got, "value")
	}
	if got := getenv("MISSING_KEY", "fallback"); got != "fallback" {
		t.Fatalf("getenv with missing key = %q, want %q", got, "fallback")
	}
}
