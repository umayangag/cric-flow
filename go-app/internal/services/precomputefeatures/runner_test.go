package precomputefeatures

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/features"
)

func TestNewRunner(t *testing.T) {
	r := NewRunner()
	require.NotNil(t, r)
}

func TestToFeatureInnings(t *testing.T) {
	t.Parallel()

	t.Run("empty input", func(t *testing.T) {
		got := toFeatureInnings(nil)
		require.Empty(t, got)
	})

	t.Run("single value", func(t *testing.T) {
		d := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
		in := []db.InnVal{{MatchDate: d, Value: 42.5}}
		got := toFeatureInnings(in)
		require.Len(t, got, 1)
		require.Equal(t, d, got[0].Date)
		require.Equal(t, 42.5, got[0].Value)
	})

	t.Run("multiple values", func(t *testing.T) {
		d1 := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
		d2 := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
		in := []db.InnVal{
			{MatchDate: d1, Value: 10},
			{MatchDate: d2, Value: 20},
		}
		got := toFeatureInnings(in)
		require.Len(t, got, 2)
		require.Equal(t, []features.Innings{
			{Date: d1, Value: 10},
			{Date: d2, Value: 20},
		}, got)
	})
}

func TestResolveConcurrencyLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested int
		wantMin   int // when requested <= 0 we only assert >= 1
		wantExact int // when > 0 we assert exact value; 0 means use wantMin only
	}{
		{"positive one", 1, 1, 1},
		{"positive five", 5, 5, 5},
		{"positive hundred", 100, 100, 100},
		{"zero uses resource limit and at least 1", 0, 1, 0},
		{"negative uses resource limit and at least 1", -1, 1, 0},
		{"large positive", 1000, 1000, 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveConcurrencyLimit(tt.requested)
			if tt.wantExact != 0 {
				require.Equal(t, tt.wantExact, got)
			} else {
				require.GreaterOrEqual(t, got, tt.wantMin)
			}
		})
	}
}

func TestReplayMatchPageSize(t *testing.T) {
	// replayMatchPageSize returns config value or default; ensure it returns a positive int
	got := replayMatchPageSize()
	require.Greater(t, got, 0, "replayMatchPageSize should return positive value")
}

func TestRunReplayGlobalPool_EmptyJobs_ReturnsNil(t *testing.T) {
	t.Parallel()
	r := NewRunner()
	ctx := context.Background()

	tests := []struct {
		name string
		jobs []FormatJob
	}{
		{"nil jobs", nil},
		{"empty jobs", []FormatJob{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := r.RunReplayGlobalPool(ctx, tt.jobs, 0, 10)
			require.NoError(t, err)
		})
	}
}
