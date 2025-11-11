package teamselect

import (
	"context"
	"errors"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teamselect"
	ts "github.com/umayangag/cric-info-scrapers/go-app/internal/services/teamselect"
)

// Runner validates options and orchestrates selection given a pre-loaded pool.
// Pool loading (CSV/DB) is performed by the caller (cmd or higher service),
// keeping this runner pure and easy to test.

type Runner struct{ Weights ts.ScoreWeights }

func NewRunner() *Runner { return &Runner{Weights: ts.DefaultWeights()} }

func (r *Runner) Run(_ context.Context, opts cli.Options, pool []ts.Player) ([]ts.Player, error) {
	if r == nil {
		return nil, errors.New("nil runner")
	}
	if opts.MatchID <= 0 || opts.Format == "" || opts.Season == "" || opts.Size < 1 || opts.MinBowlers < 0 {
		return nil, errors.New("invalid options")
	}
	if len(pool) < opts.Size {
		return nil, errors.New("insufficient pool")
	}
	team, err := ts.Select(
		pool,
		r.Weights,
		ts.Constraints{Size: opts.Size, MinBowlers: opts.MinBowlers, RequireKeeper: opts.RequireKeeper},
	)
	if err != nil {
		return nil, err
	}
	return team, nil
}
