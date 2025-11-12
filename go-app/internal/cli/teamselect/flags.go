package teamselect

import (
	"errors"
	"flag"
	"os"
	"strconv"
	"strings"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		s := strings.ToLower(strings.TrimSpace(v))
		return s == "1" || s == "true" || s == "t" || s == "yes" || s == "y"
	}
	return def
}

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
	// Seed defaults from env first
	matchID = getenvInt64("TEAM_SELECT_MATCH", 0)
	format = getenv("TEAM_SELECT_FORMAT", "T20")
	season = getenv("TEAM_SELECT_SEASON", "")
	pool = getenv("TEAM_SELECT_POOL", "")
	size = getenvInt("TEAM_SELECT_SIZE", 11)
	minB = getenvInt("TEAM_SELECT_MIN_BOWLERS", 5)
	reqK = getenvBool("TEAM_SELECT_REQUIRE_KEEPER", false)
	fromDB = getenvBool("TEAM_SELECT_FROM_DB", true)

	fs.Int64Var(&matchID, "match", matchID, "match_id to build predictions for")
	fs.StringVar(&format, "format", format, "match format code (TEST, ODI, T20, T20I)")
	fs.StringVar(&season, "season", season, "season name (e.g. 2019)")
	fs.StringVar(&pool, "pool", pool, "path to prepared pool CSV")
	fs.IntVar(&size, "size", size, "team size to select")
	fs.IntVar(&minB, "min-bowlers", minB, "minimum number of bowlers to include")
	fs.BoolVar(&reqK, "require-keeper", reqK, "require at least one wicket-keeper")
	fs.BoolVar(&fromDB, "from-db", fromDB, "build features from DB instead of CSV pool")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	format = strings.ToUpper(strings.TrimSpace(format))
	if matchID <= 0 {
		return Options{}, errors.New("invalid match")
	}
	if strings.TrimSpace(season) == "" {
		return Options{}, errors.New("season is required")
	}
	switch format {
	case "TEST", "ODI", "T20", "T20I":
		// ok
	default:
		return Options{}, errors.New("invalid format")
	}
	if size <= 0 {
		return Options{}, errors.New("invalid size")
	}
	if minB < 0 {
		return Options{}, errors.New("invalid min-bowlers")
	}
	if !fromDB && strings.TrimSpace(pool) == "" {
		return Options{}, errors.New("pool csv is required when from-db=false")
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
