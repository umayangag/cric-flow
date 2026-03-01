package predictteam

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func TestHasFieldingPredictions(t *testing.T) {
	t.Parallel()

	t.Run("empty map", func(t *testing.T) {
		require.False(t, hasFieldingPredictions(nil))
		require.False(t, hasFieldingPredictions(map[int64]PlayerPred{}))
	})

	t.Run("no fielding data", func(t *testing.T) {
		preds := map[int64]PlayerPred{
			1: {Runs: 25, Wickets: 2, Economy: 7},
			2: {Runs: 30, Wickets: 0, Economy: 0},
		}
		require.False(t, hasFieldingPredictions(preds))
	})

	t.Run("has catches", func(t *testing.T) {
		preds := map[int64]PlayerPred{
			1: {Runs: 25, Catches: 1.5},
		}
		require.True(t, hasFieldingPredictions(preds))
	})

	t.Run("has run_outs", func(t *testing.T) {
		preds := map[int64]PlayerPred{
			1: {Runs: 25, RunOuts: 0.3},
		}
		require.True(t, hasFieldingPredictions(preds))
	})
}

func TestToWeatherOverride(t *testing.T) {
	t.Parallel()

	t.Run("nil input", func(t *testing.T) {
		require.Nil(t, toWeatherOverride(nil))
	})

	t.Run("with values", func(t *testing.T) {
		w := &WeatherInput{
			Temp: 28, Humidity: 65, Wind: 10,
			Rain: 0, Cloud: 40, Pressure: 1013,
		}
		got := toWeatherOverride(w)
		require.NotNil(t, got)
		require.Equal(t, 28.0, got.Temp)
		require.Equal(t, 65.0, got.Humidity)
		require.Equal(t, 10.0, got.Wind)
		require.Equal(t, 0.0, got.Rain)
		require.Equal(t, 40.0, got.Cloud)
		require.Equal(t, 1013.0, got.Pressure)
	})
}

func TestDefaultSimulationOpts(t *testing.T) {
	t.Parallel()

	opts := DefaultSimulationOpts()
	require.Equal(t, 0, opts.TopKPerTeam)
	require.Equal(t, 0, opts.NumSamplesPerMatchup)
	require.Equal(t, int64(0), opts.Seed)
	require.Equal(t, config.DefaultSimulationRunsCV, opts.RunsCV)
	require.Equal(t, config.DefaultSimulationWicketsCV, opts.WicketsCV)
	require.Equal(t, config.DefaultSimulationEconomyCV, opts.EconomyCV)
}

func TestBuildTeamSelectPool(t *testing.T) {
	t.Parallel()

	pool := []db.PlayerPoolRow{
		{
			PlayerID:           1,
			PlayerName:         "Batter",
			IsWicketKeeper:     0,
			BattingConsistency: sql.NullFloat64{Float64: 1, Valid: true},
			BowlingConsistency: sql.NullFloat64{Float64: 0, Valid: false},
		},
		{
			PlayerID:           2,
			PlayerName:         "Bowler",
			IsWicketKeeper:     0,
			BattingConsistency: sql.NullFloat64{Float64: 0.5, Valid: true},
			BowlingConsistency: sql.NullFloat64{Float64: 0.8, Valid: true},
		},
	}
	preds := map[int64]PlayerPred{
		1: {Runs: 25, Wickets: 0, Economy: 0, Catches: 0, RunOuts: 0},
		2: {Runs: 10, Wickets: 2, Economy: 6.5, Catches: 0.5, RunOuts: 0.1},
	}
	cfg := &config.Config{}

	got := buildTeamSelectPool(pool, preds, "T20", cfg)
	require.Len(t, got, 2)
	require.Equal(t, "Batter", got[0].Name)
	require.False(t, got[0].IsBowler)
	require.Equal(t, "Bowler", got[1].Name)
	require.True(t, got[1].IsBowler)
}
