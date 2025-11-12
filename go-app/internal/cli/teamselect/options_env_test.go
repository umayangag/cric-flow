package teamselect_test

import (
	"flag"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teamselect"
)

func TestParseArgs_EnvDefaults(t *testing.T) {
	t.Parallel()
	t.Setenv("TEAM_SELECT_MATCH", "42")
	t.Setenv("TEAM_SELECT_FORMAT", "t20i")
	t.Setenv("TEAM_SELECT_SEASON", "2020")
	t.Setenv("TEAM_SELECT_SIZE", "7")
	t.Setenv("TEAM_SELECT_MIN_BOWLERS", "2")
	t.Setenv("TEAM_SELECT_REQUIRE_KEEPER", "yes")
	t.Setenv("TEAM_SELECT_FROM_DB", "0")
	t.Setenv("TEAM_SELECT_POOL", "/tmp/p.csv")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	got, err := cli.ParseArgs(fs, []string{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.MatchID != 42 || got.Format != "T20I" || got.Season != "2020" || got.TeamSize != 7 || got.MinBowlers != 2 ||
		!got.RequireKeeper ||
		got.FromDB ||
		got.PoolPath != "/tmp/p.csv" {
		t.Fatalf("unexpected parse via env: %#v", got)
	}
}
