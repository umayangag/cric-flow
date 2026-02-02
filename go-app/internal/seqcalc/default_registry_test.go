package seqcalc_test

import (
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/seqcalc"
)

func TestNewDefaultRegistry_AllTargetsAndResolveAll(t *testing.T) {
	reg := seqcalc.NewDefaultRegistry()

	gotTargets := reg.AllTargets()
	// Expected targets as declared in seqcalc.interfaces.go
	expected := map[seqcalc.Target]struct{}{
		seqcalc.TargetBatTransitions: {},
		seqcalc.TargetBowlSequences:  {},
		seqcalc.TargetPlayerWindows:  {},
		seqcalc.TargetReaction:       {},
		seqcalc.TargetDotStreaks:     {},
		seqcalc.TargetDiscipline:     {},
		seqcalc.TargetWicketModes:    {},
		seqcalc.TargetSpells:         {},
		seqcalc.TargetOverPos:        {},
		seqcalc.TargetEndPressure:    {},
	}

	if len(gotTargets) != len(expected) {
		t.Fatalf("unexpected targets count: got=%d want=%d list=%v", len(gotTargets), len(expected), gotTargets)
	}
	for _, gt := range gotTargets {
		if _, ok := expected[gt]; !ok {
			t.Fatalf("unexpected target in registry: %s", gt)
		}
	}

	// Resolve "all" should return one calculator per expected target
	calcs, err := reg.ResolveTargets("all")
	if err != nil {
		t.Fatalf("ResolveTargets(all) error: %v", err)
	}
	if len(calcs) != len(expected) {
		t.Fatalf("unexpected calculators count for all: got=%d want=%d", len(calcs), len(expected))
	}
}
