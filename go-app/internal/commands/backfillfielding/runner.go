package backfillfielding

import (
	"context"
	"errors"
	"fmt"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/backfillfielding"
	fieldingsvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/fielding"
)

// Runner orchestrates backfill-fielding execution via the fielding service.
type Runner struct {
	Svc *fieldingsvc.Service
}

func NewRunner(s *fieldingsvc.Service) *Runner { return &Runner{Svc: s} }

// Run executes the service based on CLI options.
func (r *Runner) Run(ctx context.Context, opts cli.Options) error {
	if r == nil {
		return errors.New("nil runner")
	}
	if r.Svc == nil {
		return errors.New("missing service")
	}
	if opts.All {
		_, err := r.Svc.BackfillAll(ctx, opts.Apply, opts.Concurrency)
		return err
	}
	if opts.MatchID <= 0 {
		return fmt.Errorf("invalid match id: %d", opts.MatchID)
	}
	_, err := r.Svc.BackfillMatch(ctx, opts.MatchID, opts.Apply)
	return err
}
