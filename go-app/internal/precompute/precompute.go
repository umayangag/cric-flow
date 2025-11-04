package precompute

import (
	"context"
	"fmt"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/features"
)

// Run orchestrates precompute for the given season and list of format codes.
// If season is empty, computes for all seasons. If formats is empty, computes for all formats.
func Run(parent context.Context, season string, formats []string) error {
	ctx := parent
	if d := time.Duration(config.Load().Features.PrecomputeTimeoutMs) * time.Millisecond; d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, d)
		defer cancel()
	}
	// ensure DB connection
	if db.Pool == nil {
		if _, err := db.Connect(ctx); err != nil {
			return err
		}
	}
	// Discover formats if not provided
	codes := formats
	if len(codes) == 0 {
		rows, err := db.Pool.Query(ctx, `SELECT code FROM match_format ORDER BY id`)
		if err != nil {
			return fmt.Errorf("list formats: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var code string
			if err := rows.Scan(&code); err != nil {
				return err
			}
			codes = append(codes, code)
		}
		if rows.Err() != nil {
			return rows.Err()
		}
	}

	// Update status tracker
	setStart(season, codes)
	defer setDone()

	for _, code := range codes {
		// 1) Seasonal form (weighted formulas)
		setPhase("form")
		if err := features.ComputeSeasonalFormFmt(ctx, season, code); err != nil {
			setError(err)
			return fmt.Errorf("compute seasonal form (%s): %w", code, err)
		}
		// 2) Venue effects (weighted formulas)
		setPhase("venue")
		if err := features.ComputeVenueEffectsFmt(ctx, code); err != nil {
			setError(err)
			return fmt.Errorf("compute venue effects (%s): %w", code, err)
		}
		// 3) Opposition effects (weighted formulas)
		setPhase("opposition")
		if err := features.ComputeOppositionEffectsFmt(ctx, code); err != nil {
			setError(err)
			return fmt.Errorf("compute opposition effects (%s): %w", code, err)
		}
		// 4) Consistency (mirrors Python semantics)
		setPhase("consistency")
		if err := features.ComputeConsistencyFmt(ctx, season, code); err != nil {
			setError(err)
			return fmt.Errorf("compute consistency (%s): %w", code, err)
		}
	}
	return nil
}
