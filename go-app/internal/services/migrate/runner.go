package migrate

import (
	"context"
	"time"
)

// Runner executes database migrations.
// It defers to the injected Migrate function for actual work to ease testing.
type Runner struct {
	// Migrate applies migrations found under dir.
	Migrate func(ctx context.Context, dir string) error
	// Timeout bounds the migration context; zero means default 30s.
	Timeout time.Duration
}

// Run runs migrations with a bounded context.
func (r Runner) Run(parent context.Context, dir string) error {
	to := r.Timeout
	if to <= 0 {
		to = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(parent, to)
	defer cancel()

	return r.Migrate(ctx, dir)
}
