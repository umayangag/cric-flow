package precomputeall

import (
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// ParseArgs parses CLI args into Options. Pure and testable.
// Mirrors existing flags from precompute-features and precompute-sequence-features, unified.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
	var (
		format     string
		allFormats bool
		asOf       string
		replay     bool
		alpha      float64
		lastN      int
		migrDir    string
		timeout    time.Duration
		seqTargets string
		seqDryRun  bool
	)

	defaultAlpha := config.DefaultFeatureEWMAlpha
	defaultLastN := config.DefaultFeatureConsistencyLastN
	if cfg := config.Load(); cfg != nil {
		if cfg.Features.EWMAlpha > 0 && cfg.Features.EWMAlpha <= 1 {
			defaultAlpha = float64(cfg.Features.EWMAlpha)
		}
		if cfg.Features.ConsistencyLastN > 0 {
			defaultLastN = cfg.Features.ConsistencyLastN
		}
	}
	fs.StringVar(&format, "format", "ODI", "Match format code: TEST|ODI|T20|T20I (aliases accepted: MDM, ODM, IT20); ignored if -all-formats")
	fs.BoolVar(&allFormats, "all-formats", false, "Run all canonical formats (TEST, ODI, T20I, T20) in parallel; requires -replay")
	fs.StringVar(&asOf, "as-of", "", "Cutoff date (YYYY-MM-DD); used only when -replay is false")
	fs.BoolVar(
		&replay,
		"replay",
		false,
		"Replay mode: iterate matches chronologically and write snapshots as of each match date (ignores -as-of)",
	)
	fs.Float64Var(
		&alpha,
		"ewm-alpha",
		defaultAlpha,
		"Alpha for exponentially weighted mean (0,1]; default from config features.ewm_alpha",
	)
	fs.IntVar(
		&lastN,
		"lastN",
		defaultLastN,
		"Last-N window size for consistency; default from config features.consistency_last_n",
	)
	// Default migrations dir from MIGRATIONS_DIR env if set; otherwise ./migrations
	defMig := os.Getenv("MIGRATIONS_DIR")
	if defMig == "" {
		defMig = "./migrations"
	}
	fs.StringVar(&migrDir, "migrations", defMig, "Directory with SQL migrations (can also set MIGRATIONS_DIR)")
	fs.DurationVar(&timeout, "timeout", config.DefaultTimeout, "Overall timeout for the job")

	fs.StringVar(&seqTargets, "seq-targets", "all", "Sequence targets to compute: comma-separated list or 'all'")
	fs.BoolVar(&seqDryRun, "seq-dry-run", false, "If true, list sequence computations without writing")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if !allFormats && strings.TrimSpace(format) == "" {
		return Options{}, errors.New("format must not be empty (or use -all-formats)")
	}
	if allFormats && !replay {
		return Options{}, errors.New("-all-formats requires -replay")
	}
	if !(alpha > 0 && alpha <= 1) {
		return Options{}, errors.New("ewm-alpha must be in (0,1]")
	}
	if lastN < 0 {
		return Options{}, errors.New("lastN must be >= 0")
	}
	return Options{
		Format:        format,
		AllFormats:    allFormats,
		AsOf:          asOf,
		Replay:        replay,
		EWMAlpha:      alpha,
		LastN:         lastN,
		MigrationsDir: migrDir,
		Timeout:       timeout,
		SeqTargets:    seqTargets,
		SeqDryRun:     seqDryRun,
	}, nil
}
