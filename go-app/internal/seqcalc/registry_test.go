package seqcalc

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewNoopCalculators(t *testing.T) {
	calcs := NewNoopCalculators()
	require.NotEmpty(t, calcs)
	require.GreaterOrEqual(t, len(calcs), 5)
	// All should implement Calculator and return no error
	for _, c := range calcs {
		require.NotEmpty(t, c.Name())
		err := c.Compute(context.Background(), Params{}, true)
		require.NoError(t, err)
	}
}

func TestNewRegistry(t *testing.T) {
	calcs := NewNoopCalculators()
	r := NewRegistry(calcs...)
	require.NotNil(t, r)
	targets := r.AllTargets()
	require.NotEmpty(t, targets)
	// AllTargets returns sorted
	for i := 1; i < len(targets); i++ {
		require.True(t, targets[i] > targets[i-1], "AllTargets should be sorted")
	}
}

func TestRegistry_ResolveTargets(t *testing.T) {
	calcs := NewNoopCalculators()
	r := NewRegistry(calcs...)

	tests := []struct {
		name    string
		spec    string
		wantLen int
		wantErr bool
	}{
		{"all", "all", len(calcs), false},
		{"ALL case insensitive", "ALL", len(calcs), false},
		{"empty as all", "  ", len(calcs), false},
		{"single target", "bat_transitions", 1, false},
		{"unknown target", "unknown_xyz", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.ResolveTargets(tt.spec)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, got, tt.wantLen)
		})
	}
}

func TestRegistry_ResolveTargets_CommaList(t *testing.T) {
	r := NewRegistry(NewNoopCalculators()...)
	got, err := r.ResolveTargets("bat_transitions, bowl_sequences")
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestRegistry_ResolveTargets_EmptyCommaElements(t *testing.T) {
	r := NewRegistry(NewNoopCalculators()...)
	got, err := r.ResolveTargets("bat_transitions,,bowl_sequences,")
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func TestRun_NoopCalculatorsDryRun(t *testing.T) {
	calcs := NewNoopCalculators()
	ctx := context.Background()
	params := Params{FormatCode: "T20", AsOf: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	err := Run(ctx, calcs[:2], params, true)
	require.NoError(t, err)
}

func TestRun_EmptyCalcs(t *testing.T) {
	ctx := context.Background()
	err := Run(ctx, nil, Params{}, true)
	require.NoError(t, err) // Run with empty calcs still completes (0 iterations)
}

func TestDryRun(t *testing.T) {
	calcs := NewNoopCalculators()
	r := NewRegistry(calcs...)
	resolved, _ := r.ResolveTargets("all")

	var buf bytes.Buffer
	err := DryRun(&buf, resolved, Params{FormatCode: "T20"})
	require.NoError(t, err)
	require.Contains(t, buf.String(), "format=T20")
	require.Contains(t, buf.String(), "targets=")

	// nil writer
	err = DryRun(nil, resolved, Params{})
	require.Error(t, err)

	// empty calcs
	err = DryRun(&buf, nil, Params{})
	require.Error(t, err)
}
