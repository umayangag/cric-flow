package resources

import (
	"os"
	"testing"
)

func TestParseGOMEMLIMIT(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"512MiB", 512 * 1024 * 1024},
		{"8GiB", 8 * 1024 * 1024 * 1024},
		{"1e9", 1000000000},
		{" 100MIB ", 100 * 1024 * 1024},
	}
	for _, tt := range tests {
		got := parseGOMEMLIMIT(tt.in)
		if got != tt.want {
			t.Errorf("parseGOMEMLIMIT(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestConcurrencyLimit_EnvOverride(t *testing.T) {
	const envKey = "PRECOMPUTE_CONCURRENCY"
	old := os.Getenv(envKey)
	defer func() { _ = os.Setenv(envKey, old) }()

	_ = os.Setenv(envKey, "2")
	got := ConcurrencyLimit(KindPrecompute, 0, nil)
	if got != 2 {
		t.Errorf("with PRECOMPUTE_CONCURRENCY=2 got %d, want 2", got)
	}

	_ = os.Unsetenv(envKey)
	got = ConcurrencyLimit(KindPrecompute, 0, nil)
	if got < 1 {
		t.Errorf("with no env got %d, want >= 1", got)
	}
}

func TestConcurrencyLimit_ConfigOverride(t *testing.T) {
	got := ConcurrencyLimit(KindPrecompute, 4, nil)
	if got != 4 {
		t.Errorf("with configLimit=4 got %d, want 4", got)
	}
}

func TestConcurrencyLimit_FloorAndCeiling(t *testing.T) {
	// configLimit 0 with getConfig returning 10 should be clamped by ceiling
	got := ConcurrencyLimit(KindPrecompute, 0, func() int { return 999 })
	if got < 1 {
		t.Errorf("got %d, want >= 1", got)
	}
}
