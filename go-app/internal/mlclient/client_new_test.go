package mlclient

import (
	"os"
	"testing"
	"time"
)

func TestNew_DefaultsWhenEnvUnset(t *testing.T) {
	// Ensure env unset
	_ = os.Unsetenv("ML_BASE_URL")
	c := New()
	if c.BaseURL != "http://localhost:8000" {
		t.Fatalf("BaseURL default mismatch: got %q", c.BaseURL)
	}
	if c.HTTP == nil {
		t.Fatalf("HTTP client should not be nil")
	}
	if c.UserAgent == "" {
		t.Fatalf("UserAgent should be set by default")
	}
	if c.Timeout <= 0 || c.HTTP.Timeout <= 0 {
		t.Fatalf("timeouts should be > 0, got Timeout=%v HTTP.Timeout=%v", c.Timeout, c.HTTP.Timeout)
	}
}

func TestNew_RespectsMLBaseURLEnv(t *testing.T) {
	os.Setenv("ML_BASE_URL", "http://example.test:1234")
	t.Cleanup(func() { _ = os.Unsetenv("ML_BASE_URL") })
	c := New()
	if c.BaseURL != "http://example.test:1234" {
		t.Fatalf("BaseURL env override mismatch: got %q", c.BaseURL)
	}
	// sanity
	if c.HTTP == nil {
		t.Fatalf("HTTP client should not be nil")
	}
	if c.Timeout < 1*time.Second {
		t.Fatalf("unexpected Timeout: %v", c.Timeout)
	}
}
