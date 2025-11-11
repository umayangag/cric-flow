package teampredictor

import (
	"errors"
	"flag"
	"os"
	"strconv"
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
	bat, err := strconv.Atoi(strings.TrimSpace(batStr))
	if err != nil || bat < 0 {
		return Options{}, errors.New("invalid bat")
	}
	bowl, err := strconv.Atoi(strings.TrimSpace(bowlStr))
	if err != nil || bowl < 0 {
		return Options{}, errors.New("invalid bowl")
	}
	return Options{MatchID: matchID, Format: format, Season: season, Bat: bat, Bowl: bowl}, nil
}

func getenv(k, def string) string { if v := os.Getenv(k); v != "" { return v }; return def }

func parseInt64(s string) (int64, error) { return strconv.ParseInt(strings.TrimSpace(s), 10, 64) }
