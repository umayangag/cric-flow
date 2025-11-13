package etlimporter

import (
	"context"
	"errors"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/cli/etlimporter"
	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/etlimporter"
)

// Ingestor abstracts the ingestion service for testability.
type Ingestor interface {
	IngestDir(ctx context.Context, dir, pattern string, apply bool, conc int) (svc.Stats, error)
}

// Runner orchestrates the etl-importer workflow via the ingestion service.
type Runner struct {
	Svc Ingestor
}

func NewRunner(s Ingestor) *Runner { return &Runner{Svc: s} }

// Run validates options and delegates to Service.IngestDir.
func (r *Runner) Run(ctx context.Context, opts etlimporter.Options) error {
	if r == nil {
		return errors.New("nil runner")
	}
	if r.Svc == nil {
		return errors.New("missing service")
	}
	if opts.InDir == "" {
		return errors.New("input directory is required")
	}
	if opts.Concurrency < 1 {
		return errors.New("concurrency must be >= 1")
	}
	if opts.Pattern == "" {
		return errors.New("pattern must be non-empty")
	}
	_, err := r.Svc.IngestDir(ctx, opts.InDir, opts.Pattern, opts.Apply, opts.Concurrency)
	return err
}
