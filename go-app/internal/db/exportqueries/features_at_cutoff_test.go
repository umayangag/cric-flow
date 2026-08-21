package exportqueries_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
)

func TestComputeFeaturesAtCutoffForMatch_DBUninitialized(t *testing.T) {
	// When db.defaultDB is nil, GetMatchFeatureContext returns error and we propagate it.
	db.SetDB(nil)
	defer func() { db.SetDB(nil) }()

	ctx := context.Background()
	cutoff := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	got, err := exportqueries.ComputeFeaturesAtCutoffForMatch(ctx, 100, cutoff, []int64{1, 2})
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "db pool not initialized")
}

func TestComputeFeaturesAtCutoffNoMatch_EmptyPlayerIDs(t *testing.T) {
	ctx := context.Background()
	cutoff := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	got, err := exportqueries.ComputeFeaturesAtCutoffNoMatch(ctx, cutoff, "T20", nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got)

	got, err = exportqueries.ComputeFeaturesAtCutoffNoMatch(ctx, cutoff, "ODI", []int64{})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got)
}
