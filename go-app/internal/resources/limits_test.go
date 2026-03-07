package resources

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
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
		{"1GiB", 1024 * 1024 * 1024},
		{"2MiB", 2 * 1024 * 1024},
		{"invalid", 0},
		{"0", 0},
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

func TestConcurrencyLimit_KindExport_KindFielding(t *testing.T) {
	const envExport = "EXPORT_CONCURRENCY"
	const envFielding = "FIELDING_CONCURRENCY"
	oldExport := os.Getenv(envExport)
	oldFielding := os.Getenv(envFielding)
	defer func() {
		_ = os.Setenv(envExport, oldExport)
		_ = os.Setenv(envFielding, oldFielding)
	}()

	_ = os.Setenv(envExport, "3")
	got := ConcurrencyLimit(KindExport, 0, nil)
	if got != 3 {
		t.Errorf("KindExport with EXPORT_CONCURRENCY=3 got %d, want 3", got)
	}

	_ = os.Setenv(envFielding, "4")
	got = ConcurrencyLimit(KindFielding, 0, nil)
	if got != 4 {
		t.Errorf("KindFielding with FIELDING_CONCURRENCY=4 got %d, want 4", got)
	}
}

func TestConcurrencyLimit_KindSeqCalc(t *testing.T) {
	const envKey = "SEQCALC_CONCURRENCY"
	old := os.Getenv(envKey)
	defer func() { _ = os.Setenv(envKey, old) }()
	_ = os.Setenv(envKey, "2")
	got := ConcurrencyLimit(KindSeqCalc, 0, nil)
	if got != 2 {
		t.Errorf("KindSeqCalc with SEQCALC_CONCURRENCY=2 got %d, want 2", got)
	}
}

func TestConcurrencyLimit_KindImport(t *testing.T) {
	got := ConcurrencyLimit(KindImport, 6, nil)
	if got != 6 {
		t.Errorf("KindImport with configLimit=6 got %d, want 6", got)
	}
}

// TestGetLimit_ReturnsPositive ensures GetLimit returns at least 1 for each kind (covers GetLimit and config callback path).
func TestGetLimit_ReturnsPositive(t *testing.T) {
	t.Parallel()
	for _, kind := range []Kind{KindPrecompute, KindImport, KindExport, KindSeqCalc, KindFielding} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			got := GetLimit(kind)
			require.GreaterOrEqual(t, got, 1, "GetLimit(%q) should be >= 1", kind)
		})
	}
}

func TestClampToCeiling(t *testing.T) {
	if clampToCeiling(0, 10) != 1 {
		t.Error("clampToCeiling(0,10) should floor to 1")
	}
	if clampToCeiling(5, 10) != 5 {
		t.Error("clampToCeiling(5,10) should remain 5")
	}
	if clampToCeiling(15, 10) != 10 {
		t.Error("clampToCeiling(15,10) should ceiling to 10")
	}
}

func TestMemoryBasedLimit(t *testing.T) {
	tests := []struct {
		name     string
		kind     Kind
		memLimit int64 // in bytes
		want     int
	}{
		// 2GiB * default frac (85%) / per-worker MB: precompute uses default 450 → 2048*0.85/450≈3; seqcalc capped at 1 for ≤2GB
		{"precompute 2GiB", KindPrecompute, 2 * 1024 * 1024 * 1024, 3},
		{"import 2GiB", KindImport, 2 * 1024 * 1024 * 1024, 11},
		{"seqcalc 2GiB", KindSeqCalc, 2 * 1024 * 1024 * 1024, 1},
		// 512MiB * 85% / 450 MiB < 1 → 1
		{"precompute 512MiB", KindPrecompute, 512 * 1024 * 1024, 1},
		{"no limit", KindPrecompute, 0, 0},
	}
	oldDetect := detectMemoryLimitBytes
	defer func() { detectMemoryLimitBytes = oldDetect }()
	// Use constants (disable observations) so test is deterministic.
	oldObs := os.Getenv("USE_RESOURCE_OBSERVATIONS")
	defer func() { _ = os.Setenv("USE_RESOURCE_OBSERVATIONS", oldObs) }()
	_ = os.Setenv("USE_RESOURCE_OBSERVATIONS", "false")

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
