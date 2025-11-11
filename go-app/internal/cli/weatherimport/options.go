package weatherimport

import (
	"errors"
	"flag"
	"os"
	"strconv"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// Options captures CLI options for weather-import.
// Only CLI-derived options live here; higher layers may merge config/env.
// Behavior is preserved to act as a thin delegator.
type Options struct {
	MatchID   int64
	Provider  string
	Apply     bool
}

// ParseArgs parses flags using the provided FlagSet and argument slice.
// It does not exit the process on error; instead it returns an error for tests to assert on.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var (
		matchStr string
		provider string
		apply    bool
	)

	// Defaults from env/config
	defProvider := getenv("WEATHER_PROVIDER", "dummy")
	if defProvider == "" {
		defProvider = "dummy"
	}
	_ = config.Load() // ensure config cache ready for potential future defaults

	fs.StringVar(&matchStr, "match", "", "match id (required)")
	fs.StringVar(&provider, "provider", defProvider, "weather provider name (e.g., dummy)")
	fs.BoolVar(&apply, "apply", false, "apply changes; if false, dry-run")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	var matchID int64
	if strings.TrimSpace(matchStr) == "" {
		return Options{}, errors.New("match id is required")
	}
	if m, err := strconv.ParseInt(strings.TrimSpace(matchStr), 10, 64); err != nil || m <= 0 {
		return Options{}, errors.New("invalid match id")
	} else {
		matchID = m
	}

	provider = strings.TrimSpace(provider)
	if provider == "" {
		return Options{}, errors.New("provider is required")
	}

	return Options{MatchID: matchID, Provider: provider, Apply: apply}, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
