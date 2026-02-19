package teampredictor

import (
	"context"
	"errors"

	cli "github.com/umayangag/cric-flow/go-app/internal/cli/teampredictor"
	"github.com/umayangag/cric-flow/go-app/internal/mlclient"
)

// Runner validates options and delegates to the team-predictor service.
type Runner struct{ service Predictor }

func NewRunner(s Predictor) *Runner { return &Runner{service: s} }

func (r *Runner) Run(ctx context.Context, opts cli.Options) (mlclient.PredictResponse, error) {
	if r == nil {
		return mlclient.PredictResponse{}, errors.New("nil runner")
	}
	if r.service == nil {
		return mlclient.PredictResponse{}, errors.New("missing service")
	}
	if opts.MatchID <= 0 || opts.Format == "" || opts.Season == "" || opts.Bat < 0 || opts.Bowl < 0 {
		return mlclient.PredictResponse{}, errors.New("invalid options")
	}
	return r.service.Predict(ctx, opts)
}
