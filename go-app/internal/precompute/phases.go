package precompute

import (
	"context"
	"fmt"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/features"
)

// runPhasesForFormat executes all precompute phases for a single format code.
// It updates the in-memory status for each phase and preserves existing error
// messages and wrapping.
func runPhasesForFormat(ctx context.Context, season string, code string) error {
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
	return nil
}
