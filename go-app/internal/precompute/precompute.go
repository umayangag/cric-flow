// Package precompute orchestrates precompute phases and maintains in-memory
// status for long-running feature computations. Pipeline precompute uses the
// snapshot tables (feature_form_snapshots, feature_consistency_snapshots) via
// the precompute-features runner; the legacy _fmt tables are no longer used.
package precompute

import (
	"context"
	"log/slog"

	pfcmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/precomputefeatures"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// RunOpts holds optional overrides for Run. Nil or zero values mean use config.
type RunOpts struct {
	Alpha float64 // (0,1] to override config; else use config
	LastN int     // > 0 to override config; else use config
}

// Run orchestrates precompute for the given season and list of format codes.
// It populates feature_form_snapshots and feature_consistency_snapshots (and
// triggers sequence features) per format. If formats is empty, computes for all formats.
// Season is ignored; the snapshot runner replays all matches chronologically.
// Pass nil for opts to use config for alpha and lastN.
// Run uses the parent context as-is; no extra deadline is applied here.
// Callers (API pipeline.RunJob or CLI precompute-all) set the timeout (e.g. 24h or 0 for no limit).
func Run(parent context.Context, season string, formats []string, opts *RunOpts) (err error) {
	ctx := parent
	cfg := config.Load()
	if db.Pool == nil {
		slog.Info("precompute: connecting to database (pool was nil)")
		if _, connectErr := db.Connect(ctx); connectErr != nil {
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
	slog.Info("precompute: starting run", slog.String("season", season), slog.Any("format_codes", codes))

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

	runner := pfcmd.NewRunner()
	for _, code := range codes {
		setPhase("form")
		slog.Info("precompute: starting format", slog.String("format", code))
		formatID, err := db.GetMatchFormatIDByCode(ctx, code)
		if err != nil {
			slog.Error("precompute: get format ID failed", slog.String("format", code), slog.Any("err", err))
			return err
		}
		if err := runner.RunReplay(ctx, code, formatID, alpha, lastN, windowN); err != nil {
			slog.Error(
				"precompute: RunReplay failed",
				slog.String("format", code),
				slog.Int64("format_id", formatID),
				slog.Any("err", err),
			)
			return err
		}
		slog.Info("precompute: format completed", slog.String("format", code))
	}
	return nil
}
