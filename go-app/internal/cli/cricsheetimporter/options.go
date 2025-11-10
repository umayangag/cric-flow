package cricsheetimporter

import (
	"errors"
	"flag"
	"os"
	"strconv"
	"strings"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
)

// Options captures CLI options for cricsheet-importer.
// Only CLI-derived options live here; higher layers may merge config/env.
type Options struct {
	InDir       string
	Apply       bool
	Concurrency int
}

// ParseArgs parses flags using the provided FlagSet and argument slice.
// It does not exit the process on error; instead it returns an error for tests to assert on.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var inDir string
	var apply bool
	var concurrency int

	// Environment defaults
	defIn := getenv("GO_APP_CRICSHEET_DIR", "")
	if defIn == "" {
		defIn = config.DefaultCricsheetDir()
	}
	defConc := getenvInt("CRICSHEET_CONCURRENCY", 4)

	fs.StringVar(&inDir, "in", defIn, "input directory containing Cricsheet match files")
	fs.BoolVar(&apply, "apply", false, "apply changes (upsert to DB); if false, dry-run")
	fs.IntVar(&concurrency, "concurrency", defConc, "number of concurrent workers")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	if strings.TrimSpace(inDir) == "" {
		return Options{}, errors.New("input directory is required")
	}
	if concurrency < 1 {
		return Options{}, errors.New("concurrency must be >= 1")
	}

	return Options{InDir: inDir, Apply: apply, Concurrency: concurrency}, nil
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
