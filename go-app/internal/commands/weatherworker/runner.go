package weatherworker

import (
	"context"
	"errors"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/weatherworker"
)

// Service abstracts the weather worker service for testability.
// It mirrors the Run method we need from the underlying service.
type Service interface {
	Run(ctx context.Context, maxJobs int, apply bool) (int, error)
}

// Runner validates CLI options and delegates execution to the Service.
// It is small and testable; no logging here.
type Runner struct {
	Svc Service
}

func NewRunner(s Service) *Runner { return &Runner{Svc: s} }

func (r *Runner) Run(ctx context.Context, opts cli.Options) error {
	if r == nil {
		return errors.New("nil runner")
	}
	if r.Svc == nil {
		return errors.New("missing service")
	}
	if opts.Provider == "" {
		return errors.New("provider is required")
	}
	if opts.MaxJobs < 0 {
		return errors.New("invalid max")
	}
	_, err := r.Svc.Run(ctx, opts.MaxJobs, opts.Apply)
	return err
}
