package resources

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGOMEMLIMIT(t *testing.T) {
	testCases := []struct {
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
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.in, func(t *testing.T) {
			got := parseGOMEMLIMIT(tc.in)
			require.Equal(t, tc.want, got, "parseGOMEMLIMIT(%q)", tc.in)
		})
	}
}

func TestConcurrencyLimit_EnvOverride(t *testing.T) {
	const envKey = "IMPORT_CONCURRENCY"
	old := os.Getenv(envKey)
	defer func() { _ = os.Setenv(envKey, old) }()

	_ = os.Setenv(envKey, "2")
	got := ConcurrencyLimit(KindImport, 0, nil)
	require.Equal(t, 2, got, "with IMPORT_CONCURRENCY=2")
	// Env must win over config callback: config says 8, env says 2 → 2
	got = ConcurrencyLimit(KindImport, 0, func() int { return 8 })
	require.Equal(t, 2, got, "env should override config callback")

	_ = os.Unsetenv(envKey)
	got = ConcurrencyLimit(KindImport, 0, nil)
	require.GreaterOrEqual(t, got, 1, "with no env")
}

func TestConcurrencyLimit_ConfigOverride(t *testing.T) {
	got := ConcurrencyLimit(KindImport, 4, nil)
	require.Equal(t, 4, got, "with configLimit=4")
}

func TestConcurrencyLimit_FloorAndCeiling(t *testing.T) {
	// configLimit 0 with getConfig returning 999 should be clamped by ceiling
	const highValue = 999
	got := ConcurrencyLimit(KindImport, 0, func() int { return highValue })
	require.GreaterOrEqual(t, got, 1, "floor to 1")
	require.Less(t, got, highValue, "clamped by ceiling")
}

func TestConcurrencyLimit_KindImport(t *testing.T) {
	got := ConcurrencyLimit(KindImport, 6, nil)
	require.Equal(t, 6, got, "KindImport with configLimit=6")
}

// TestGetLimit_ReturnsPositive ensures GetLimit returns at least 1 (covers GetLimit and
// the config callback path).
func TestGetLimit_ReturnsPositive(t *testing.T) {
	t.Parallel()
	require.GreaterOrEqual(t, GetLimit(KindImport), 1, "GetLimit(import) should be >= 1")
}

func TestClampToCeiling(t *testing.T) {
	require.Equal(t, 1, clampToCeiling(0, 10), "floor to 1")
	require.Equal(t, 5, clampToCeiling(5, 10), "unchanged")
	require.Equal(t, 10, clampToCeiling(15, 10), "ceiling to 10")
}

func TestMemoryBasedLimit(t *testing.T) {
	testCases := []struct {
		name     string
		kind     Kind
		memLimit int64 // in bytes
		want     int
	}{
		// 2GiB * default frac (85%) / 150 MB per import worker ≈ 11
		{"import 2GiB", KindImport, 2 * 1024 * 1024 * 1024, 11},
		// 128MiB * 85% / 150 MB < 1 → 1
		{"import 128MiB", KindImport, 128 * 1024 * 1024, 1},
		{"no limit", KindImport, 0, 0},
	}
	oldDetect := detectMemoryLimitBytes
	defer func() { detectMemoryLimitBytes = oldDetect }()
	// Use constants (disable observations) so test is deterministic.
	oldObs := os.Getenv("USE_RESOURCE_OBSERVATIONS")
	defer func() { _ = os.Setenv("USE_RESOURCE_OBSERVATIONS", oldObs) }()
	_ = os.Setenv("USE_RESOURCE_OBSERVATIONS", "false")

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			detectMemoryLimitBytes = func() int64 { return tc.memLimit }
			got := memoryBasedLimit(tc.kind)
			require.Equal(t, tc.want, got, "memoryBasedLimit()")
		})
	}
}
