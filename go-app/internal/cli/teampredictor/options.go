package teampredictor

import (
	"errors"
	"flag"
	"os"
	"strings"
)

// Options captures CLI options for team-predictor.
type Options struct {
	MatchID int64
	Format  string
	Season  string
	Bat     int
	Bowl    int
}

// ParseArgs parses flags using the provided FlagSet and argument slice.
// It returns an error instead of exiting so tests can assert on it.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var matchStr, format, season string
	var batStr, bowlStr string

	defFormat := getenv("TEAM_PREDICTOR_FORMAT", "")
	defSeason := getenv("TEAM_PREDICTOR_SEASON", "")

	fs.StringVar(&matchStr, "match", getenv("TEAM_PREDICTOR_MATCH", ""), "match id (int64)")
	fs.StringVar(&format, "format", defFormat, "format code (TEST|ODI|T20I|T20)")
	fs.StringVar(&season, "season", defSeason, "season (e.g., 2019)")
	fs.StringVar(&batStr, "bat", getenv("TEAM_PREDICTOR_BAT", "0"), "number of batters (>=0)")
	fs.StringVar(&bowlStr, "bowl", getenv("TEAM_PREDICTOR_BOWL", "0"), "number of bowlers (>=0)")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	matchID, err := parsePositiveInt64ForMatch(matchStr)
	if err != nil {
		return Options{}, err
	}
	if format, err = normalizeFormat(format); err != nil {
		return Options{}, err
	}
	season = strings.TrimSpace(season)
	if season == "" {
		return Options{}, errors.New("season is required")
	}
	bat, err := parseNonNegativeInt("bat", batStr)
	if err != nil {
		return Options{}, err
	}
	bowl, err := parseNonNegativeInt("bowl", bowlStr)
	if err != nil {
		return Options{}, err
	}
	return Options{MatchID: matchID, Format: format, Season: season, Bat: bat, Bowl: bowl}, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// parseInt64 was unused; removed to satisfy lint.
