package etlimporter

import (
	"errors"
	"flag"
	"os"
	"strconv"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// Options captures CLI options for etl-importer.
// Only CLI-derived options live here; higher layers may merge config/env.
type Options struct {
	InDir       string
	Apply       bool
	Concurrency int
	Pattern     string
}

// ParseArgs parses flags using the provided FlagSet and argument slice.
// It does not exit the process on error; instead it returns an error for tests to assert on.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var inDir string
	var apply bool
	var concurrency int
	var pattern string

	// Environment defaults
	defIn := getenv("GO_APP_ETL_DIR", "")
	if strings.TrimSpace(defIn) == "" {
		defIn = config.DefaultEtlDir()
	}
	defConc := getenvInt("ETL_CONCURRENCY", 4)
	defPattern := getenv("ETL_PATTERN", "*.csv")

	fs.StringVar(&inDir, "in", defIn, "input directory containing curated CSV files")
	fs.BoolVar(&apply, "apply", false, "apply changes (upsert to DB); if false, dry-run")
	fs.IntVar(&concurrency, "concurrency", defConc, "number of concurrent workers (>=1)")
	fs.StringVar(&pattern, "pattern", defPattern, "glob pattern to select files (e.g., *.csv)")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	if strings.TrimSpace(inDir) == "" {
		return Options{}, errors.New("input directory is required")
	}
	if concurrency < 1 {
		return Options{}, errors.New("concurrency must be >= 1")
	}
	if strings.TrimSpace(pattern) == "" {
		return Options{}, errors.New("pattern must be non-empty")
	}

	return Options{InDir: inDir, Apply: apply, Concurrency: concurrency, Pattern: pattern}, nil
}

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
