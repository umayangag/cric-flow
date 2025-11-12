package teamselect

import (
	"errors"
	"flag"
	"strings"
)

// ParseArgs parses CLI args into Options. Pure and testable.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var (
		matchID int64
		format  string
		season  string
		pool    string
		size    int
		minB    int
		reqK    bool
		fromDB  bool
	)
	fs.Int64Var(&matchID, "match", 0, "match_id to build predictions for")
	fs.StringVar(&format, "format", "T20", "match format code (TEST, ODI, T20, T20I)")
	fs.StringVar(&season, "season", "", "season name (e.g. 2019)")
	fs.StringVar(&pool, "pool", "../ml-service/ml/pool.csv", "path to prepared pool CSV")
	fs.IntVar(&size, "size", 11, "team size to select")
	fs.IntVar(&minB, "min-bowlers", 5, "minimum number of bowlers to include")
	fs.BoolVar(&reqK, "require-keeper", false, "require at least one wicket-keeper")
	fs.BoolVar(&fromDB, "from-db", true, "build features from DB instead of CSV pool")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if matchID == 0 || strings.TrimSpace(season) == "" {
		return Options{}, errors.New(
			"usage: team-select -match=<id> -season=<name> [-format=CODE] [-pool=path] [-size=N] [-min-bowlers=M] [--require-keeper] [--from-db=true|false]",
		)
	}
	return Options{
		MatchID:       matchID,
		Format:        format,
		Season:        season,
		PoolPath:      pool,
		TeamSize:      size,
		MinBowlers:    minB,
		RequireKeeper: reqK,
		FromDB:        fromDB,
	}, nil
}
