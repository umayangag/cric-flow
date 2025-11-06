package main

import (
	"errors"
	"flag"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// options represents parsed CLI inputs for team-select.
type options struct {
	matchID       int64
	formatCode    string
	seasonName    string
	poolPath      string
	teamSize      int
	minBowlers    int
	requireKeeper bool
	fromDB        bool
}

// parseFlags parses CLI args into options, applying sensible defaults.
// This function is pure and does not read environment variables or files
// (aside from using provided cfg for any future defaults; currently not required).
func parseFlags(args []string, cfg *config.Config) (options, error) {
	var (
		matchID int64
		format  string
		season  string
		pool    string
		size    int
		minB    int
		reqK    bool
		fromDB bool
	)

	fs := flag.NewFlagSet("team-select", flag.ContinueOnError)
	fs.Int64Var(&matchID, "match", 0, "match_id to build predictions for")
	fs.StringVar(&format, "format", "T20", "match format code (TEST, ODI, T20, T20I)")
	fs.StringVar(&season, "season", "", "season name (e.g. 2019)")
	fs.StringVar(&pool, "pool", "../ml-service/ml/pool.csv", "path to prepared pool CSV")
	fs.IntVar(&size, "size", 11, "team size to select")
	fs.IntVar(&minB, "min-bowlers", 5, "minimum number of bowlers to include")
	fs.BoolVar(&reqK, "require-keeper", false, "require at least one wicket-keeper")
	fs.BoolVar(&fromDB, "from-db", true, "build features from DB instead of CSV pool")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}

	if matchID == 0 || strings.TrimSpace(season) == "" {
		return options{}, errors.New("usage: team-select -match=<id> -season=<name> [-format=CODE] [-pool=path] [-size=N] [-min-bowlers=M] [--require-keeper] [--from-db=true|false]")
	}

	// Future: apply cfg-based defaults/validation if needed. For now we honor flags and built-ins above.
	return options{
		matchID:       matchID,
		formatCode:    format,
		seasonName:    season,
		poolPath:      pool,
		teamSize:      size,
		minBowlers:    minB,
		requireKeeper: reqK,
		fromDB:        fromDB,
	}, nil
}
