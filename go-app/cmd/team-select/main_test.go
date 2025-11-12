package main

import (
	"flag"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teamselect"
)

// These tests validate the CLI parsing used by main.go via internal/cli/teamselect.
func TestParseArgs_DefaultsAndOverrides(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want cli.Options
	}{
		{
			name: "defaults applied (format T20, fromDB=true, size=11, minBowlers=5)",
			args: []string{"-match=262039498036", "-season=2025"},
			want: cli.Options{
				MatchID:    262039498036,
				Format:     "T20",
				Season:     "2025",
				TeamSize:   11,
				MinBowlers: 5,
				FromDB:     true,
			},
		},
		{
			name: "overrides respected (ODI, csv pool, keeper)",
			args: []string{
				"-match=1", "-season=2019", "-format=ODI", "-pool=/tmp/pool.csv",
				"-size=9", "-min-bowlers=4", "-require-keeper", "-from-db=false",
			},
			want: cli.Options{
				MatchID:       1,
				Format:        "ODI",
				Season:        "2019",
				PoolPath:      "/tmp/pool.csv",
				TeamSize:      9,
				MinBowlers:    4,
				RequireKeeper: true,
				FromDB:        false,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			got, err := cli.ParseArgs(fs, tc.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.MatchID != tc.want.MatchID || got.Format != tc.want.Format || got.Season != tc.want.Season ||
				got.TeamSize != tc.want.TeamSize || got.MinBowlers != tc.want.MinBowlers ||
				got.RequireKeeper != tc.want.RequireKeeper || got.FromDB != tc.want.FromDB ||
				got.PoolPath != tc.want.PoolPath {
				t.Fatalf("options mismatch\n got: %#v\nwant: %#v", got, tc.want)
			}
		})
	}
}

func TestParseArgs_Errors(t *testing.T) {
	t.Parallel()
	bad := [][]string{
		{},
		{"-season=2025"},
		{"-match=1"},
		{"-match=1", "-format=X", "-season=2019"},
	}
	for _, args := range bad {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		if _, err := cli.ParseArgs(fs, args); err == nil {
			t.Fatalf("expected error for args: %v", args)
		}
	}
}
