package precomputeall

import "time"

// Options holds CLI flags for the unified precompute-all command.
type Options struct {
    // Common
    Format        string
    AsOf          string // YYYY-MM-DD or empty
    Replay        bool
    EWMAlpha      float64
    LastN         int
    MigrationsDir string
    Timeout       time.Duration

    // Sequence specific
    SeqTargets string // comma-separated or "all"
    SeqDryRun  bool
}
