package teamselect

import (
	"errors"
	"flag"
	"os"
	"strconv"
	"strings"
)

// Options captures CLI options for team-select.
type Options struct {
	MatchID       int64
	Format        string
	Season        string
	Size          int
	MinBowlers    int
	RequireKeeper bool
	FromDB        bool
	PoolCSV       string
}

// ParseArgs parses flags using the provided FlagSet and argument slice.
// It returns an error instead of exiting so tests can assert on it.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var (
		matchStr string
		format   string
		season   string
		sizeStr  string
		minBStr string
		reqK     bool
		fromDB  bool
		pool    string
	)

	fs.StringVar(&matchStr, "match", getenv("TEAM_SELECT_MATCH", ""), "match id (int64)")
	fs.StringVar(&format, "format", getenv("TEAM_SELECT_FORMAT", ""), "format code (TEST|ODI|T20I|T20)")
	fs.StringVar(&season, "season", getenv("TEAM_SELECT_SEASON", ""), "season (e.g., 2019)")
	fs.StringVar(&sizeStr, "size", getenv("TEAM_SELECT_SIZE", "11"), "team size (>=1)")
	fs.StringVar(&minBStr, "min-bowlers", getenv("TEAM_SELECT_MIN_BOWLERS", "0"), "minimum bowlers (>=0)")
	fs.BoolVar(&reqK, "require-keeper", getenvBool("TEAM_SELECT_REQUIRE_KEEPER", false), "require wicket-keeper")
	fs.BoolVar(&fromDB, "from-db", getenvBool("TEAM_SELECT_FROM_DB", true), "load pool from DB (true) or CSV (false)")
	fs.StringVar(&pool, "pool", getenv("TEAM_SELECT_POOL", ""), "pool CSV path when --from-db=false")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	matchID, err := parseInt64(matchStr)
	if err != nil || matchID <= 0 {
		return Options{}, errors.New("invalid match id")
	}
	format = strings.ToUpper(strings.TrimSpace(format))
	switch format {
	case "TEST", "ODI", "T20I", "T20":
	default:
		return Options{}, errors.New("invalid format")
	}
	season = strings.TrimSpace(season)
	if season == "" {
		return Options{}, errors.New("season is required")
	}
	size, err := strconv.Atoi(strings.TrimSpace(sizeStr))
	if err != nil || size < 1 {
		return Options{}, errors.New("invalid size")
	}
	minB, err := strconv.Atoi(strings.TrimSpace(minBStr))
	if err != nil || minB < 0 {
		return Options{}, errors.New("invalid min-bowlers")
	}
	if !fromDB {
		pool = strings.TrimSpace(pool)
		if pool == "" {
			return Options{}, errors.New("pool csv required when --from-db=false")
		}
	}
	return Options{
		MatchID: matchID,
		Format: format,
		Season: season,
		Size: size,
		MinBowlers: minB,
		RequireKeeper: reqK,
		FromDB: fromDB,
		PoolCSV: pool,
	}, nil
}

func getenv(k, def string) string { if v := os.Getenv(k); v != "" { return v }; return def }

func getenvBool(k string, def bool) bool {
	if v := os.Getenv(k); v != "" {
		lv := strings.ToLower(strings.TrimSpace(v))
		return lv == "1" || lv == "true" || lv == "yes"
	}
	return def
}

func parseInt64(s string) (int64, error) { return strconv.ParseInt(strings.TrimSpace(s), 10, 64) }
