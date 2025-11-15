package seqcalc

import (
	"context"
	"time"
)

// Target represents a calculator target key.
type Target string

const (
	TargetAll            Target = "all"
	TargetBatTransitions Target = "bat_transitions"
	TargetBowlSequences  Target = "bowl_sequences"
	TargetPlayerWindows  Target = "player_windows"
	TargetReaction       Target = "reaction"
	TargetDotStreaks     Target = "dot_streaks"
	TargetDiscipline     Target = "discipline"
	TargetWicketModes    Target = "wicket_modes"
	TargetSpells         Target = "spells"
	TargetOverPos        Target = "overpos"
	TargetEndPressure    Target = "end_pressure"
)

// Calculator is a minimal interface all feature calculators will implement.
// For the scaffolding step (1.5), implementations are no-ops.
type Calculator interface {
	// Name returns the stable target name.
	Name() Target
	// Compute runs the calculator for the given scope. Implementations must be idempotent.
	// When dryRun is true, implementations MUST NOT write to the DB and may only enumerate work.
	Compute(ctx context.Context, params Params, dryRun bool) error
}

// Params captures common inputs for calculators.
type Params struct {
	// AsOf is the date boundary for latest-as-of logic. Zero means "use match date" semantics by calculators.
	AsOf time.Time
	// FormatCode is one of T20|ODI|TEST (case-insensitive); calculators may map it to format_id internally.
	FormatCode string
	// MatchIDs optionally restrict computation to specific matches; empty means all available.
	MatchIDs []int64
}
