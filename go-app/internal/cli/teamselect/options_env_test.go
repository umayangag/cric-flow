package teamselect_test

import (
	"flag"
	"os"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teamselect"
)

func TestParseArgs_EnvDefaults(t *testing.T) {
	t.Parallel()
	os.Setenv("TEAM_SELECT_MATCH", "42")
	os.Setenv("TEAM_SELECT_FORMAT", "t20i")
	os.Setenv("TEAM_SELECT_SEASON", "2020")
	os.Setenv("TEAM_SELECT_SIZE", "7")
	os.Setenv("TEAM_SELECT_MIN_BOWLERS", "2")
	os.Setenv("TEAM_SELECT_REQUIRE_KEEPER", "yes")
	os.Setenv("TEAM_SELECT_FROM_DB", "0")
	os.Setenv("TEAM_SELECT_POOL", "/tmp/p.csv")
	t.Cleanup(func() {
		os.Unsetenv("TEAM_SELECT_MATCH")
		os.Unsetenv("TEAM_SELECT_FORMAT")
		os.Unsetenv("TEAM_SELECT_SEASON")
		os.Unsetenv("TEAM_SELECT_SIZE")
		os.Unsetenv("TEAM_SELECT_MIN_BOWLERS")
		os.Unsetenv("TEAM_SELECT_REQUIRE_KEEPER")
		os.Unsetenv("TEAM_SELECT_FROM_DB")
		os.Unsetenv("TEAM_SELECT_POOL")
	})
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	got, err := cli.ParseArgs(fs, []string{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.MatchID != 42 || got.Format != "T20I" || got.Season != "2020" || got.Size != 7 || got.MinBowlers != 2 ||
		!got.RequireKeeper ||
		got.FromDB ||
		got.PoolCSV != "/tmp/p.csv" {
		t.Fatalf("unexpected parse via env: %#v", got)
	}
}
