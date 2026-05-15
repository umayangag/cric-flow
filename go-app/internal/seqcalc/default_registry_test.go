package seqcalc_test

import (
	"bytes"
	"testing"

	"github.com/umayangag/cric-flow/go-app/internal/seqcalc"

	"github.com/stretchr/testify/require"
)

func TestRegistry_ResolveTargets_SingleAndUnknown(t *testing.T) {
	reg := seqcalc.NewDefaultRegistry()

	calcs, err := reg.ResolveTargets("bat_transitions")
	require.NoError(t, err)
	if len(calcs) != 1 {
		t.Fatalf("expected 1 calc, got %d", len(calcs))
	}
	if calcs[0].Name() != seqcalc.TargetBatTransitions {
		t.Fatalf("expected bat_transitions, got %s", calcs[0].Name())
	}

	_, err = reg.ResolveTargets("unknown_target_xyz")
	require.Error(t, err)
}

func TestDryRun(t *testing.T) {
	reg := seqcalc.NewDefaultRegistry()
	calcs, _ := reg.ResolveTargets("bat_transitions")

	var buf bytes.Buffer
	err := seqcalc.DryRun(&buf, calcs, seqcalc.Params{FormatCode: "T20"})
	require.NoError(t, err)
	if buf.Len() == 0 {
		t.Fatal("DryRun should write output")
	}

	// Error: nil writer
	err = seqcalc.DryRun(nil, calcs, seqcalc.Params{})
	require.Error(t, err)

	// Error: no calculators
	err = seqcalc.DryRun(&buf, nil, seqcalc.Params{})
	require.Error(t, err)
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
	require.NoError(t, err)
	if len(calcs) != len(expected) {
		t.Fatalf("unexpected calculators count for all: got=%d want=%d", len(calcs), len(expected))
	}
}
