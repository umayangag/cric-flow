package precomputefeatures

import "time"

// Options holds CLI flags for precompute-features.
type Options struct {
	Format        string
	AsOf          string // YYYY-MM-DD or empty
	Replay        bool
	EWMAlpha      float64
	LastN         int
	MigrationsDir string
	Timeout       time.Duration
}
