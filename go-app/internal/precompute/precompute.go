// Package precompute orchestrates precompute phases and maintains in-memory
// status for long-running feature computations. Pipeline precompute uses the
// snapshot tables (feature_form_snapshots, feature_consistency_snapshots) via
// the precompute-features runner; the legacy _fmt tables are no longer used.
package precompute

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/sync/errgroup"

	pfcmd "github.com/umayangag/cric-flow/go-app/internal/commands/precomputefeatures"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/resources"
)

// Package-level function variables allow tests to replace external dependencies.
var (
	loadConfig          = config.Load
	connectDB           = db.Connect
	getFormatIDByCode   = db.GetMatchFormatIDByCode
	getResourceLimit    = resources.GetLimit
	newRunner           = func() replayRunner { return pfcmd.NewRunner() }
)

// replayRunner abstracts the RunReplay method for testability.
type replayRunner interface {
	RunReplay(ctx context.Context, code string, formatID int64, alpha float64, lastN, windowN, limit int) error
}

// RunOpts holds optional overrides for Run. Nil or zero values mean use config.
type RunOpts struct {
	Alpha float64 // (0,1] to override config; else use config
	LastN int     // > 0 to override config; else use config
}

// Run orchestrates precompute for the given season and list of format codes.
// It populates feature_form_snapshots and feature_consistency_snapshots (and
// triggers sequence features) per format. If formats is empty, computes for all formats.
// Multiple formats run in parallel, sharing the resource-aware concurrency limit (80%
// of available memory/CPU from config/env); each format gets an equal share of workers.
// Season is ignored; the snapshot runner replays all matches chronologically.
// Pass nil for opts to use config for alpha and lastN.
// Run uses the parent context as-is; no extra deadline is applied here.
// Callers (API pipeline.RunJob or CLI precompute-all) set the timeout (e.g. 24h or 0 for no limit).
func Run(parent context.Context, season string, formats []string, opts *RunOpts) (err error) {
	ctx := parent
	cfg := loadConfig()
	if !db.Available() && db.Pool == nil {
		slog.Info("precompute: connecting to database (pool was nil)")
		if _, connectErr := connectDB(ctx); connectErr != nil {
			slog.Error("precompute: database connect failed", slog.Any("err", connectErr))
			return connectErr
		}
	}
	codes, err := discoverFormatCodes(ctx, formats)
	if err != nil {
		slog.Error(
			"precompute: discover format codes failed",
			slog.Any("err", err),
			slog.Any("formats_requested", formats),
		)
		return err
	}
	if len(codes) == 0 {
		return nil
	}
	slog.Info("precompute: starting run", slog.String("season", season), slog.Any("format_codes", codes))

	// Log resource context to diagnose OOM: memory limit env, heap, and goroutine count.
	if gomemlimit := os.Getenv("GOMEMLIMIT"); gomemlimit != "" {
		slog.Info("precompute: GOMEMLIMIT", slog.String("value", gomemlimit))
	}
	resources.LogMemoryAndGoroutines("precompute: memory and goroutines at start")

	setStart(season, codes)
	defer func() {
		if err != nil {
			setLastError(err.Error())
		}
		setDone()
	}()

	alpha := config.DefaultFeatureEWMAlpha
	if opts != nil && opts.Alpha > 0 && opts.Alpha <= 1 {
		alpha = opts.Alpha
	} else if cfg.Features.EWMAlpha > 0 && cfg.Features.EWMAlpha <= 1 {
		alpha = cfg.Features.EWMAlpha
	}
	lastN := config.DefaultFeatureConsistencyLastN
	if opts != nil && opts.LastN > 0 {
		lastN = opts.LastN
	} else if cfg.Features.ConsistencyLastN > 0 {
		lastN = cfg.Features.ConsistencyLastN
	}
	windowN := 0
	if cfg.Features.HistoryWindowMatches > 0 {
		windowN = cfg.Features.HistoryWindowMatches
	}

	// Resource-aware limit (from env/config: 80% of available memory/CPU). Split across formats when running in parallel.
	totalLimit := getResourceLimit(resources.KindPrecompute)
	if totalLimit < 1 {
		totalLimit = 1
	}
	perFormatLimit := totalLimit / len(codes)
	if perFormatLimit < 1 {
		perFormatLimit = 1
	}
	slog.Info("precompute: running formats",
		slog.Int("formats", len(codes)),
		slog.Int("total_concurrency", totalLimit),
		slog.Int("per_format_concurrency", perFormatLimit),
	)

	// Resolve format IDs before starting parallel work so we fail fast on invalid codes.
	type formatJob struct {
		code     string
		formatID int64
	}
	jobs := make([]formatJob, 0, len(codes))
	for _, code := range codes {
		formatID, idErr := getFormatIDByCode(ctx, code)
		if idErr != nil {
			slog.Error("precompute: get format ID failed", slog.String("format", code), slog.Any("err", idErr))
			return idErr
		}
		jobs = append(jobs, formatJob{code: code, formatID: formatID})
	}

	setPhase("form")
	setCurrentFormat(strings.Join(codes, ", "))

	runner := newRunner()
	g, gCtx := errgroup.WithContext(ctx)
	for _, job := range jobs {
		job := job
		g.Go(func() error {
			slog.Info("precompute: starting format", slog.String("format", job.code))
			if runErr := runner.RunReplay(gCtx, job.code, job.formatID, alpha, lastN, windowN, perFormatLimit); runErr != nil {
				slog.Error(
					"precompute: RunReplay failed",
					slog.String("format", job.code),
					slog.Int64("format_id", job.formatID),
					slog.Any("err", runErr),
				)
				return runErr
			}
			resources.LogMemoryAndGoroutines("precompute: format completed", slog.String("format", job.code))
			return nil
		})
	}
	return g.Wait()
}
