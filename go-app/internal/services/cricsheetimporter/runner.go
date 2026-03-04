package cricsheetimporter

import (
	"context"
	"errors"
	"fmt"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/logger"
	"github.com/umayangag/cric-flow/go-app/internal/models"
)

// Runner orchestrates the cricsheet-importer workflow.
type Runner struct {
	Loader     cricsheet.Loader
	Parser     cricsheet.Parser
	Repository db.MatchRepo
	Log        logger.Logger
}

// NewRunner constructs a Runner with its dependencies.
func NewRunner(
	loader cricsheet.Loader,
	parser cricsheet.Parser,
	repo db.MatchRepo,
	log logger.Logger,
) *Runner {
	return &Runner{Loader: loader, Parser: parser, Repository: repo, Log: log}
}

// Run executes the import according to options.
func (r *Runner) Run(ctx context.Context, opts Options) error {
	if r == nil {
		return errors.New("nil runner")
	}
	if r.Loader == nil || r.Parser == nil || r.Repository == nil {
		return errors.New("missing dependency: Loader/Parser/Repository required")
	}
	if opts.InDir == "" {
		return errors.New("input directory is required")
	}

	ids, err := r.Loader.List(ctx, opts.InDir)
	if err != nil {
		return fmt.Errorf("list inputs: %w", err)
	}
	if r.Log != nil {
		r.Log.Infof(ctx, "found %d cricsheet files", len(ids))
	}

	var batch []models.Match
	for _, id := range ids {
		raw, lerr := r.Loader.Load(ctx, opts.InDir, id)
		if lerr != nil {
			return fmt.Errorf("load %s: %w", id, lerr)
		}
		matches, perr := r.Parser.Parse(ctx, raw)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", id, perr)
		}
		if len(matches) > 0 {
			batch = append(batch, matches...)
		}
	}

	if !opts.Apply {
		if r.Log != nil {
			r.Log.Infof(ctx, "dry-run: parsed %d matches", len(batch))
		}
		return nil
	}

	if err := r.Repository.UpsertMatches(ctx, batch); err != nil {
		return fmt.Errorf("upsert matches: %w", err)
	}
	if r.Log != nil {
		r.Log.Infof(ctx, "upserted %d matches", len(batch))
	}
	return nil
}
