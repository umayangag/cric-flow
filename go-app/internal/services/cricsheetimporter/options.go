package cricsheetimporter

import (
	"errors"
	"flag"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
)

// Options captures CLI options for cricsheet-importer.
// Only CLI-derived options live here; higher layers may merge config/env.
type Options struct {
	InDir                string
	Concurrency          int
	PlaceholdersFielding bool
	FailFast             bool
	Timeout              time.Duration
}

// ParseArgs parses flags using the provided FlagSet and argument slice.
// It does not exit the process on error; instead it returns an error for tests to assert on.
//
// There is no "-apply" / dry-run flag: every run writes, and IMPORT-03 made writes
// delete-then-insert rather than idempotent upserts, so a flag that only pretended to gate
// that would be actively dangerous (IMPORT-10). An operator who needs to preview a run should
// point -in at a scratch database, not expect this CLI to no-op against the real one.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var inDir string
	var concurrency int

	// The dataset directory resolves the same way for the CLI as for the API.
	defIn := dataset.Dir()
	defConc := getenvInt("CRICSHEET_CONCURRENCY", runtime.NumCPU())

	fs.StringVar(&inDir, "in", defIn, "input directory containing Cricsheet match files")
	fs.IntVar(&concurrency, "concurrency", defConc, "number of concurrent workers")
	// Legacy behavior flags retained for parity with existing CLI
	var placeholdersFielding bool
	var failFast bool
	fs.BoolVar(
		&placeholdersFielding,
		"placeholders-fielding",
		false,
		"insert zeroed fielding rows for all players seen",
	)
	fs.BoolVar(&failFast, "fail-fast", true, "abort on first file or DB error (default: true)")

	var timeout time.Duration
	fs.DurationVar(&timeout, "timeout", config.DefaultTimeout, "operation timeout")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}

	if strings.TrimSpace(inDir) == "" {
		return Options{}, errors.New("input directory is required")
	}
	if concurrency < 1 {
		return Options{}, errors.New("concurrency must be >= 1")
	}

	return Options{
		InDir:                inDir,
		Concurrency:          concurrency,
		PlaceholdersFielding: placeholdersFielding,
		FailFast:             failFast,
		Timeout:              timeout,
	}, nil
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
