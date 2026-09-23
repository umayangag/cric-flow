package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEntityCache_WithNoPool_ReportsTheFailureInsteadOfAnID is IMPORT-13. Every lookup
// here used to answer (0, nil) when there was no pool, as a convenience for tests, which
// swallowed the "db pool not initialized" the repository function underneath already
// returned. The importer wrote that zero into format_id, venue_id, season_id, player_id
// and opposition_id, and nothing anywhere said a lookup had failed. A lookup that cannot
// reach its table has not found a zero.
func TestEntityCache_WithNoPool_ReportsTheFailureInsteadOfAnID(t *testing.T) {
	origPool, origPoolAPI := Pool, PoolAPI
	Pool, PoolAPI = nil, nil
	t.Cleanup(func() { Pool, PoolAPI = origPool, origPoolAPI })

	ctx := context.Background()
	// Not the global cache: an entry another test warmed would be a hit and never reach
	// the pool this case is about.
	cache := &EntityCache{}

	testCases := []struct {
		name   string
		lookup func() (int64, error)
	}{
		{
			name:   "a player",
			lookup: func() (int64, error) { return cache.GetPlayerID(ctx, "abc12345", "any", "2024-01-01") },
		},
		{
			name:   "a venue",
			lookup: func() (int64, error) { return cache.GetVenueID(ctx, "any", "any city") },
		},
		{
			name:   "a season",
			lookup: func() (int64, error) { return cache.GetSeasonID(ctx, "2024") },
		},
		{
			name:   "a format",
			lookup: func() (int64, error) { return cache.GetFormatID(ctx, "T20") },
		},
		{
			name:   "an opposition",
			lookup: func() (int64, error) { return cache.GetOppositionID(ctx, "India", "male") },
		},
	}
	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			id, err := testCase.lookup()

			require.Error(t, err, "a lookup with no database behind it has failed")
			require.Contains(t, err.Error(), "pool not initialized")
			require.Zero(t, id)
		})
	}
}

// A bucket's format ids cannot be read without a database either, and the failure is the
// first lookup's.
func TestEntityCache_GetFormatIDsForTrainingBucket_WithNoPool_ReportsTheFailure(t *testing.T) {
	origPool, origPoolAPI := Pool, PoolAPI
	Pool, PoolAPI = nil, nil
	t.Cleanup(func() { Pool, PoolAPI = origPool, origPoolAPI })

	ids, err := (&EntityCache{}).GetFormatIDsForTrainingBucket(context.Background(), "T20")

	require.Error(t, err)
	require.Contains(t, err.Error(), "pool not initialized")
	require.Nil(t, ids)
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

// TestFormatCodesForTrainingBucket_NamesTheCodesTheBucketCovers pins the naming rule on
// its own, without a database: T20 and T20I are one training bucket however the caller
// spells or spaces the name, and every other format is its own.
func TestFormatCodesForTrainingBucket_NamesTheCodesTheBucketCovers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		format    string
		wantCodes []string
	}{
		{name: "T20 uppercase", format: "T20", wantCodes: []string{"T20", "T20I"}},
		{name: "T20I uppercase", format: "T20I", wantCodes: []string{"T20", "T20I"}},
		{name: "t20 lowercase", format: "t20", wantCodes: []string{"T20", "T20I"}},
		{name: "t20i lowercase", format: "t20i", wantCodes: []string{"T20", "T20I"}},
		{name: "T20 with surrounding space", format: "  T20  ", wantCodes: []string{"T20", "T20I"}},
		{name: "ODI single format", format: "ODI", wantCodes: []string{"ODI"}},
		{name: "TEST single format", format: "TEST", wantCodes: []string{"TEST"}},
		{name: "odi lowercase", format: "odi", wantCodes: []string{"ODI"}},
		{name: "odI mixed case", format: "odI", wantCodes: []string{"ODI"}},
		{name: "whitespace only trimmed odi", format: "  odi  ", wantCodes: []string{"ODI"}},
	}
	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			codes := FormatCodesForTrainingBucket(testCase.format)

			require.Equal(t, testCase.wantCodes, codes)
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

func TestPlayerKey_ExternalIDAndNameNeverCollide(t *testing.T) {
	t.Parallel()

	// The whole point of the identity work: a registry identifier and a name are
	// different kinds of key and must not share a namespace.
	require.NotEqual(t, playerKey("SR Taylor", ""), playerKey("", "SR Taylor"))
	require.Equal(t, playerKey("92cf79a8", "SR Taylor"), playerKey("92cf79a8", "S Taylor"))
	require.NotEqual(t, playerKey("92cf79a8", "SR Taylor"), playerKey("a9231c3f", "SR Taylor"))
}

func TestOppositionKey_SameNameDifferentGenderAreDifferentTeams(t *testing.T) {
	t.Parallel()

	require.NotEqual(t, oppositionKey("Australia", "male"), oppositionKey("Australia", "female"))
	// The separator has to be a character no team name contains, or "Indi"+"a" and
	// "India"+"" would be one team.
	require.NotEqual(t, oppositionKey("Indi", "a"), oppositionKey("India", ""))
}
