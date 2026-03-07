package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEntityCache_WhenPoolNil(t *testing.T) {
	// Ensure Pool is nil for these tests (unit test without DB)
	origPool := Pool
	Pool = nil
	t.Cleanup(func() { Pool = origPool })

	ctx := context.Background()
	cache := GetGlobalCache()

	// When Pool is nil, cache returns 0, nil (fallback for tests)
	id, err := cache.GetPlayerID(ctx, "any")
	require.NoError(t, err)
	require.Equal(t, int64(0), id)

	id, err = cache.GetVenueID(ctx, "any")
	require.NoError(t, err)
	require.Equal(t, int64(0), id)

	id, err = cache.GetSeasonID(ctx, "2024")
	require.NoError(t, err)
	require.Equal(t, int64(0), id)

	id, err = cache.GetFormatID(ctx, "T20")
	require.NoError(t, err)
	require.Equal(t, int64(0), id)

	ids, err := cache.GetFormatIDsForTrainingBucket(ctx, "T20")
	require.NoError(t, err)
	require.Len(t, ids, 2) // T20 and T20I

	ids, err = cache.GetFormatIDsForTrainingBucket(ctx, "ODI")
	require.NoError(t, err)
	require.Len(t, ids, 1)

	id, err = cache.GetOppositionID(ctx, "India")
	require.NoError(t, err)
	require.Equal(t, int64(0), id)
}

func TestEntityCache_WarmupPlayers_WhenPoolNil(t *testing.T) {
	origPool := Pool
	Pool = nil
	t.Cleanup(func() { Pool = origPool })

	ctx := context.Background()
	cache := GetGlobalCache()

	err := cache.WarmupPlayers(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "pool not initialized")
}

// TestEntityCache_GetFormatIDsForTrainingBucket_FormatNormalization verifies that
// format string trimming/case and T20/T20I bucket logic behave correctly when Pool is nil.
func TestEntityCache_GetFormatIDsForTrainingBucket_FormatNormalization(t *testing.T) {
	origPool := Pool
	Pool = nil
	t.Cleanup(func() { Pool = origPool })

	ctx := context.Background()
	cache := GetGlobalCache()

	tests := []struct {
		name           string
		format         string
		wantNumFormats int // T20/T20I bucket returns 2 IDs; others return 1
	}{
		{"T20 uppercase", "T20", 2},
		{"T20I uppercase", "T20I", 2},
		{"t20 lowercase", "t20", 2},
		{"t20i lowercase", "t20i", 2},
		{"T20 with surrounding space", "  T20  ", 2},
		{"ODI single format", "ODI", 1},
		{"TEST single format", "TEST", 1},
		{"odi lowercase", "odi", 1},
		{"odI mixed case", "odI", 1},
		{"whitespace only trimmed odi", "  odi  ", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := cache.GetFormatIDsForTrainingBucket(ctx, tt.format)
			require.NoError(t, err)
			require.Len(
				t,
				ids,
				tt.wantNumFormats,
				"format %q should yield %d format ID(s)",
				tt.format,
				tt.wantNumFormats,
			)
		})
	}
}

// TestGetGlobalCache_Singleton verifies that GetGlobalCache returns the same instance each time.
func TestGetGlobalCache_Singleton(t *testing.T) {
	t.Parallel()
	c1 := GetGlobalCache()
	c2 := GetGlobalCache()
	require.Same(t, c1, c2)
}
