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
		{"100B", 100},
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
	// Env must win over config callback: config says 8, env says 2 → 2
	got = ConcurrencyLimit(KindPrecompute, 0, func() int { return 8 })
	if got != 2 {
		t.Errorf("env should override config callback: got %d, want 2", got)
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
	// configLimit 0 with getConfig returning 999 should be clamped by ceiling
	const highValue = 999
	got := ConcurrencyLimit(KindPrecompute, 0, func() int { return highValue })
	if got < 1 {
		t.Errorf("got %d, want >= 1", got)
	}
	if got >= highValue {
		t.Errorf("got %d, want value to be clamped by ceiling (less than %d)", got, highValue)
	}
}

func TestMemoryBasedLimit(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		memLimit int64 // in bytes
		want     int
	}{
		{"precompute 2GiB", KindPrecompute, 2 * 1024 * 1024 * 1024, 7},
		{"import 2GiB", KindImport, 2 * 1024 * 1024 * 1024, 9},
		{"seqcalc 2GiB", KindSeqCalc, 2 * 1024 * 1024 * 1024, 7},
		{"precompute 512MiB", KindPrecompute, 512 * 1024 * 1024, 1},
		{"no limit", KindPrecompute, 0, 0},
	}
	oldDetect := detectMemoryLimitBytes
	defer func() { detectMemoryLimitBytes = oldDetect }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detectMemoryLimitBytes = func() int64 { return tt.memLimit }
			got := memoryBasedLimit(tt.kind)
			if got != tt.want {
				t.Errorf("memoryBasedLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}
