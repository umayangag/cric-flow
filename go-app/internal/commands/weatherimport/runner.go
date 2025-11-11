package weatherimport

import (
	"context"
	"errors"
	"fmt"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/weatherimport"
)

// Ingestor is the minimal interface the runner depends on (facilitates testing).
// It matches the method from the weather import service.
type Ingestor interface {
	Import(ctx context.Context, matchID int64, apply bool) (int, error)
}

// Runner orchestrates weather-import by delegating to a small service.
type Runner struct {
	Svc Ingestor
}

func NewRunner(s Ingestor) *Runner { return &Runner{Svc: s} }

// Run validates options and delegates to the service.
func (r *Runner) Run(ctx context.Context, opts cli.Options) error {
	if r == nil {
		return errors.New("nil runner")
	}
	if r.Svc == nil {
		return errors.New("missing service")
	}
	if opts.MatchID <= 0 {
		return fmt.Errorf("invalid match id: %d", opts.MatchID)
	}
	_, err := r.Svc.Import(ctx, opts.MatchID, opts.Apply)
	return err
}
