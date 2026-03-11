package teamselect_test

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

func TestParseArgs_EnvDefaults(t *testing.T) {
	t.Setenv("TEAM_SELECT_MATCH", "42")
	t.Setenv("TEAM_SELECT_FORMAT", "t20i")
	t.Setenv("TEAM_SELECT_SEASON", "2020")
	t.Setenv("TEAM_SELECT_SIZE", "7")
	t.Setenv("TEAM_SELECT_MIN_BOWLERS", "2")
	t.Setenv("TEAM_SELECT_REQUIRE_KEEPER", "yes")
	t.Setenv("TEAM_SELECT_FROM_DB", "0")
	t.Setenv("TEAM_SELECT_POOL", "/tmp/p.csv")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	got, err := svc.ParseArgs(fs, []string{})
	require.NoError(t, err)
	require.Equal(t, int64(42), got.MatchID)
	require.Equal(t, "T20I", got.Format)
	require.Equal(t, "2020", got.Season)
	require.Equal(t, 7, got.TeamSize)
	require.Equal(t, 2, got.MinBowlers)
	require.True(t, got.RequireKeeper)
	require.False(t, got.FromDB)
	require.Equal(t, "/tmp/p.csv", got.PoolPath)
}
