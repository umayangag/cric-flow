// Package precompute orchestrates precompute phases and maintains in-memory
// status for long-running feature computations. Pipeline precompute uses the
// snapshot tables (feature_form_snapshots, feature_consistency_snapshots) via
// the precompute-features runner; the legacy _fmt tables are no longer used.
package precompute

import (
	"context"
	"time"

	pfcmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/precomputefeatures"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

const defaultEWMAlpha = 0.3
const defaultConsistencyLastN = 10

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
func Run(parent context.Context, season string, formats []string, opts *RunOpts) error {
	ctx := parent
	cfg := config.Load()
	if d := time.Duration(cfg.Features.PrecomputeTimeoutMs) * time.Millisecond; d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, d)
		defer cancel()
	}
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	codes, err := discoverFormatCodes(ctx, formats)
	if err != nil {
		return err
	}

	setStart(season, codes)
	defer setDone()

	alpha := defaultEWMAlpha
	if opts != nil && opts.Alpha > 0 && opts.Alpha <= 1 {
		alpha = opts.Alpha
	} else if cfg.Features.EWMAlpha > 0 && cfg.Features.EWMAlpha <= 1 {
		alpha = cfg.Features.EWMAlpha
	}
	lastN := defaultConsistencyLastN
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
		formatID, err := db.GetMatchFormatIDByCode(ctx, code)
		if err != nil {
			return err
		}
		if err := runner.RunReplay(ctx, code, formatID, alpha, lastN, windowN); err != nil {
			return err
		}
	}
	return nil
}
