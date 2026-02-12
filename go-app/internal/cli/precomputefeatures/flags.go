package precomputefeatures

import (
	"errors"
	"flag"
	"os"
	"strings"
	"time"
)

// ParseArgs parses CLI args into Options. Pure and testable.
// Behavior parity with previous cmd implementation:
// - format defaults to ODI
// - as-of is optional; when empty, caller may default to today (UTC)
// - alpha must be in (0,1]
// - lastN must be >= 0
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var (
		format  string
		asOf    string
		replay  bool
		alpha   float64
		lastN   int
		migrDir string
		timeout time.Duration
	)
	fs.StringVar(&format, "format", "ODI", "Match format code: TEST|ODI|T20|T20I (aliases accepted: MDM, ODM, IT20)")
	fs.StringVar(&asOf, "as-of", "", "Cutoff date (YYYY-MM-DD); used only when -replay is false")
	fs.BoolVar(
		&replay,
		"replay",
		false,
		"Replay mode: iterate matches chronologically and write snapshots as of each match date (ignores -as-of)",
	)
	fs.Float64Var(&alpha, "ewm-alpha", 0.3, "Alpha for exponentially weighted mean (0,1]")
	fs.IntVar(&lastN, "lastN", 10, "Last-N window size for consistency")
	// Default migrations dir from MIGRATIONS_DIR env if set; otherwise ./migrations
	defMig := os.Getenv("MIGRATIONS_DIR")
	if defMig == "" {
		defMig = "./migrations"
	}
	fs.StringVar(&migrDir, "migrations", defMig, "Directory with SQL migrations (can also set MIGRATIONS_DIR)")
	fs.DurationVar(&timeout, "timeout", 5*time.Hour, "Overall timeout for the job")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if strings.TrimSpace(format) == "" {
		return Options{}, errors.New("format must not be empty")
	}
	if !(alpha > 0 && alpha <= 1) {
		return Options{}, errors.New("ewm-alpha must be in (0,1]")
	}
	if lastN < 0 {
		return Options{}, errors.New("lastN must be >= 0")
	}
	// no strict validation for as-of to preserve legacy behavior
	return Options{
		Format:        format,
		AsOf:          asOf,
		Replay:        replay,
		EWMAlpha:      alpha,
		LastN:         lastN,
		MigrationsDir: migrDir,
		Timeout:       timeout,
	}, nil
}
