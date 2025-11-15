package seqcalc

import (
	"context"
)

// Below are no-op calculator implementations for scaffolding. They only satisfy the interface
// and allow wiring and dry-run listing. Actual logic will be added in subsequent sub-plans.

type baseNoop struct{ name Target }

func (b baseNoop) Name() Target                                      { return b.name }
func (b baseNoop) Compute(_ context.Context, _ Params, _ bool) error { return nil }

func NewNoopCalculators() []Calculator {
	return []Calculator{
		baseNoop{name: TargetBatTransitions},
		baseNoop{name: TargetBowlSequences},
		baseNoop{name: TargetPlayerWindows},
		baseNoop{name: TargetReaction},
		baseNoop{name: TargetDotStreaks},
		baseNoop{name: TargetDiscipline},
		baseNoop{name: TargetWicketModes},
		baseNoop{name: TargetSpells},
		baseNoop{name: TargetOverPos},
		baseNoop{name: TargetEndPressure},
	}
}
