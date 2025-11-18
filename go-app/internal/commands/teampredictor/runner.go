package teampredictor

import (
	"context"
	"errors"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teampredictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
)

// Predictor abstracts the service behavior used by this command.
type Predictor interface {
	Predict(ctx context.Context, opts cli.Options) (mlclient.PredictResponse, error)
}

// Runner validates options and delegates to the team-predictor service.

type Runner struct{ Svc Predictor }

func NewRunner(s Predictor) *Runner { return &Runner{Svc: s} }

func (r *Runner) Run(ctx context.Context, opts cli.Options) (mlclient.PredictResponse, error) {
	if r == nil {
		return mlclient.PredictResponse{}, errors.New("nil runner")
	}
	if r.Svc == nil {
		return mlclient.PredictResponse{}, errors.New("missing service")
	}
	if opts.MatchID <= 0 || opts.Format == "" || opts.Season == "" || opts.Bat < 0 || opts.Bowl < 0 {
		return mlclient.PredictResponse{}, errors.New("invalid options")
	}
	return r.Svc.Predict(ctx, opts)
}
