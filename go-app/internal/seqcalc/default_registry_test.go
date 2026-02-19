package seqcalc_test

import (
	"bytes"
	"testing"

	"github.com/umayangag/cric-flow/go-app/internal/seqcalc"
)

func TestRegistry_ResolveTargets_SingleAndUnknown(t *testing.T) {
	reg := seqcalc.NewDefaultRegistry()

	calcs, err := reg.ResolveTargets("bat_transitions")
	if err != nil {
		t.Fatalf("ResolveTargets(bat_transitions) error: %v", err)
	}
	if len(calcs) != 1 {
		t.Fatalf("expected 1 calc, got %d", len(calcs))
	}
	if calcs[0].Name() != seqcalc.TargetBatTransitions {
		t.Fatalf("expected bat_transitions, got %s", calcs[0].Name())
	}

	_, err = reg.ResolveTargets("unknown_target_xyz")
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
}

func TestDryRun(t *testing.T) {
	reg := seqcalc.NewDefaultRegistry()
	calcs, _ := reg.ResolveTargets("bat_transitions")

	var buf bytes.Buffer
	err := seqcalc.DryRun(&buf, calcs, seqcalc.Params{FormatCode: "T20"})
	if err != nil {
		t.Fatalf("DryRun error: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("DryRun should write output")
	}

	// Error: nil writer
	err = seqcalc.DryRun(nil, calcs, seqcalc.Params{})
	if err == nil {
		t.Fatal("expected error for nil writer")
	}

	// Error: no calculators
	err = seqcalc.DryRun(&buf, nil, seqcalc.Params{})
	if err == nil {
		t.Fatal("expected error for empty calcs")
	}
}

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
