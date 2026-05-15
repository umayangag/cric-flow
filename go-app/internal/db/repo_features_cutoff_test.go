package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// DefaultFeatureProvider no longer returns average-based features; it returns an error so callers
// use precomputed path. These tests assert that behavior.

func TestDefaultFeatureProvider_EmptyPlayerIDs_ReturnsEmptyMap(t *testing.T) {
	p := &DefaultFeatureProvider{}
	cutoff := time.Date(2024, 10, 30, 0, 0, 0, 0, time.UTC)
	got, err := p.GetPlayerFeaturesAtCutoff(context.Background(), cutoff, nil)
	require.NoError(t, err)
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(got))
	}
	got, err = p.GetPlayerFeaturesAtCutoff(context.Background(), cutoff, []int64{})
	require.NoError(t, err)
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(got))
	}
}

func TestDefaultFeatureProvider_NonEmptyPlayerIDs_ReturnsErrorNoAverages(t *testing.T) {
	p := &DefaultFeatureProvider{}
	cutoff := time.Date(2024, 10, 30, 0, 0, 0, 0, time.UTC)
	got, err := p.GetPlayerFeaturesAtCutoff(context.Background(), cutoff, []int64{1})
	if err == nil {
		t.Fatalf("expected error (precomputed only, no averages), got nil and %d features", len(got))
	}
	if !contains(err.Error(), "precomputed") {
		t.Fatalf("error should mention precomputed: %v", err)
	}
}

func TestDefaultFeatureProvider_ZeroCutoff_ReturnsError(t *testing.T) {
	p := &DefaultFeatureProvider{}
	got, err := p.GetPlayerFeaturesAtCutoff(context.Background(), time.Time{}, []int64{1})
	require.Error(t, err)
	require.Nil(t, got)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	L, l := len(s), len(sub)
	if l == 0 {
		return 0
	}
	for i := 0; i <= L-l; i++ {
		if s[i:i+l] == sub {
			return i
		}
	}
	return -1
}
