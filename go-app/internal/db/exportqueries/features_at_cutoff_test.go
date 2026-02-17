package exportqueries

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

func TestComputeFeaturesAtCutoffForMatch_DBUninitialized(t *testing.T) {
	// When db.defaultDB is nil, GetMatchFeatureContext returns error and we propagate it.
	db.SetDB(nil)
	defer func() { db.SetDB(nil) }()

	ctx := context.Background()
	cutoff := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	got, err := ComputeFeaturesAtCutoffForMatch(ctx, 100, cutoff, []int64{1, 2})
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "db pool not initialized")
}
