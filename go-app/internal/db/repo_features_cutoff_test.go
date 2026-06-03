package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func TestDefaultFeatureProvider_EmptyPlayerIDs_ReturnsEmptyMap(t *testing.T) {
	p := &db.DefaultFeatureProvider{}
	cutoff := time.Date(2024, 10, 30, 0, 0, 0, 0, time.UTC)

	got, err := p.GetPlayerFeaturesAtCutoff(context.Background(), cutoff, nil)
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = p.GetPlayerFeaturesAtCutoff(context.Background(), cutoff, []int64{})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDefaultFeatureProvider_NonEmptyPlayerIDs_ReturnsErrorNoAverages(t *testing.T) {
	p := &db.DefaultFeatureProvider{}
	cutoff := time.Date(2024, 10, 30, 0, 0, 0, 0, time.UTC)

	_, err := p.GetPlayerFeaturesAtCutoff(context.Background(), cutoff, []int64{1})
	require.Error(t, err)
	require.Contains(t, err.Error(), "precomputed")
}

func TestDefaultFeatureProvider_ZeroCutoff_ReturnsError(t *testing.T) {
	p := &db.DefaultFeatureProvider{}

	got, err := p.GetPlayerFeaturesAtCutoff(context.Background(), time.Time{}, []int64{1})
	require.Error(t, err)
	require.Nil(t, got)
}
