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
