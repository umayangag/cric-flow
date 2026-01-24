package precomputeall

import (
    "errors"
    "flag"
    "strings"
    "time"
)

// ParseArgs parses CLI args into Options. Pure and testable.
// Mirrors existing flags from precompute-features and precompute-sequence-features, unified.
func ParseArgs(fs *flag.FlagSet, args []string) (Options, error) {
    var (
        format  string
        asOf    string
        replay  bool
        alpha   float64
        lastN   int
        migrDir string
        timeout time.Duration
        seqTargets string
        seqDryRun  bool
    )

    fs.StringVar(&format, "format", "ODI", "Match format code: TEST|ODI|T20|T20I (aliases accepted: MDM, ODM, IT20)")
    fs.StringVar(&asOf, "as-of", "", "Cutoff date (YYYY-MM-DD); used only when -replay is false")
    fs.BoolVar(&replay, "replay", false, "Replay mode: iterate matches chronologically and write snapshots as of each match date (ignores -as-of)")
    fs.Float64Var(&alpha, "ewm-alpha", 0.3, "Alpha for exponentially weighted mean (0,1]")
    fs.IntVar(&lastN, "lastN", 10, "Last-N window size for consistency")
    fs.StringVar(&migrDir, "migrations", "./migrations", "Directory with SQL migrations")
    fs.DurationVar(&timeout, "timeout", 30*time.Minute, "Overall timeout for the job")

    fs.StringVar(&seqTargets, "seq-targets", "all", "Sequence targets to compute: comma-separated list or 'all'")
    fs.BoolVar(&seqDryRun, "seq-dry-run", false, "If true, list sequence computations without writing")

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
    return Options{
        Format:        format,
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
