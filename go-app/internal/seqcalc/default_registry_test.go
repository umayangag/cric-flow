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
	require.Len(t, calcs, 1)
	require.Equal(t, seqcalc.TargetBatTransitions, calcs[0].Name())

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

	require.Len(t, gotTargets, len(expected))
	for _, gt := range gotTargets {
		require.Contains(t, expected, gt, "unexpected target in registry: %s", gt)
	}

	// Resolve "all" should return one calculator per expected target
	calcs, err := reg.ResolveTargets("all")
	require.NoError(t, err)
	require.Len(t, calcs, len(expected))
}
