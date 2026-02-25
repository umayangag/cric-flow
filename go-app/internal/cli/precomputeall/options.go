package precomputeall

import "time"

// Options holds CLI flags for the unified precompute-all command.
type Options struct {
	// Common
	Format        string
	AllFormats    bool   // run all canonical formats in parallel (replay only)
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
