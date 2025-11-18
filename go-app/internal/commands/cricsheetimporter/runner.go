package cricsheetimporter

import (
	"context"
	"errors"
	"fmt"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/fsx"
	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/cricsheetimporter"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
)

// Runner orchestrates the cricsheet-importer workflow.
type Runner struct {
	FS         fsx.FS
	Loader     cricsheet.Loader
	Parser     cricsheet.Parser
	Repository db.MatchRepo
	Log        logger.Logger
}

// NewRunner constructs a Runner with its dependencies.
func NewRunner(
	fs fsx.FS,
	loader cricsheet.Loader,
	parser cricsheet.Parser,
	repo db.MatchRepo,
	log logger.Logger,
) *Runner {
	return &Runner{FS: fs, Loader: loader, Parser: parser, Repository: repo, Log: log}
}

// Run executes the import according to options.
// For now, it performs a simple sequential iteration over inputs. Concurrency
// may be added in a follow-up without changing the public contract.
func (r *Runner) Run(ctx context.Context, opts cli.Options) error {
	if r == nil {
		return errors.New("nil runner")
	}
	if r.FS == nil || r.Loader == nil || r.Parser == nil || r.Repository == nil {
		return errors.New("missing dependency: FS/Loader/Parser/Repository required")
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
