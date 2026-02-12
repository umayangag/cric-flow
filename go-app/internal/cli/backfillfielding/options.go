package backfillfielding

import (
	"errors"
	"flag"
	"os"
	"strconv"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// Options captures CLI options for backfill-fielding.
// Only CLI-derived options live here; higher layers may merge config/env.
type Options struct {
	All         bool
	MatchID     int64
	Apply       bool
	Concurrency int
	Timeout     time.Duration
}

// ParseArgs parses flags using the provided FlagSet and argument slice.
// It does not exit the process on error; instead it returns an error for tests to assert on.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var (
		all         bool
		matchStr    string
		apply       bool
		concurrency int
		timeout     time.Duration
	)

	defConc := getenvInt("BACKFILL_CONCURRENCY", 4)

	fs.BoolVar(&all, "all", false, "process all matches")
	fs.StringVar(&matchStr, "match", "", "process a single match id (int)")
	fs.BoolVar(&apply, "apply", false, "apply changes; if false, dry-run")
	fs.IntVar(&concurrency, "concurrency", defConc, "number of concurrent workers (>=1)")
	fs.DurationVar(&timeout, "timeout", config.DefaultTimeout, "operation timeout")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	var matchID int64
	if matchStr != "" {
		// accept decimal numbers only; keep error explicit for tests
		m, err := strconv.ParseInt(matchStr, 10, 64)
		if err != nil || m <= 0 {
			return Options{}, errors.New("invalid match id")
		}
		matchID = m
	}

	if !all && matchID == 0 {
		return Options{}, errors.New("either --all or --match is required")
	}
	if all && matchID != 0 {
		return Options{}, errors.New("provide only one of --all or --match")
	}
	if concurrency < 1 {
		return Options{}, errors.New("concurrency must be >= 1")
	}

	return Options{All: all, MatchID: matchID, Apply: apply, Concurrency: concurrency, Timeout: timeout}, nil
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
