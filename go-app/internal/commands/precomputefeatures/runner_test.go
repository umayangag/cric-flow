package precomputefeatures

import (
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

	t.Run("positive requested uses value", func(t *testing.T) {
		require.Equal(t, 5, resolveConcurrencyLimit(5))
		require.Equal(t, 1, resolveConcurrencyLimit(1))
		require.Equal(t, 100, resolveConcurrencyLimit(100))
	})

	t.Run("zero or negative uses resource limit and ensures at least 1", func(t *testing.T) {
		got := resolveConcurrencyLimit(0)
		require.GreaterOrEqual(t, got, 1, "resolveConcurrencyLimit(0) should return >= 1")
	})
}
