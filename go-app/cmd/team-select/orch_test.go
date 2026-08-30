package main_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/predictor"
	"github.com/umayangag/cric-flow/go-app/internal/selection"
	ts "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

// These are orchestration-style tests that live next to the cmd but use the public Runner API.
// They ensure the thin wiring connects, prints the expected output, and surfaces failures.

type fakeConnector struct {
	err    error
	called int
}

func (f *fakeConnector) Connect(_ context.Context) error {
	f.called++
	return f.err
}

type fakeSelector struct {
	res selection.Result
	err error
}

func (f fakeSelector) SelectTeam(
	_ context.Context,
	_ int64,
	_ string,
	_ selection.Options,
) (selection.Result, error) {
	return f.res, f.err
}

func sampleResult() selection.Result {
	return selection.Result{
		Players: []predictor.PlayerPrediction{
			{PlayerName: "A", WinningProbability: 0.9},
			{PlayerName: "B", WinningProbability: 0.8},
		},
		TeamWinProbability: 0.7777,
	}
}

func TestOrch_Table(t *testing.T) {
	t.Parallel()

	type arrangeFn func() (r ts.Runner, opts ts.Options, buf *bytes.Buffer, fc *fakeConnector)
	type assertFn func(t *testing.T, buf *bytes.Buffer, err error, fc *fakeConnector)

	testCases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "success prints team and connects once",
			arrange: func() (ts.Runner, ts.Options, *bytes.Buffer, *fakeConnector) {
				fs := fakeSelector{res: sampleResult()}
				fc := &fakeConnector{}
				r := ts.NewRunner(fs, fc)
				buf := &bytes.Buffer{}
				opts := ts.Options{
					MatchID:    1,
					Format:     "T20",
					Season:     "2025",
					TeamSize:   11,
					MinBowlers: 5,
				}
				return r, opts, buf, fc
			},
			assert: func(t *testing.T, buf *bytes.Buffer, err error, fc *fakeConnector) {
				require.NoError(t, err)
				require.Equal(t, 1, fc.called)
				out := buf.String()
				require.Contains(t, out, "Selected Team (size=2)")
				require.Contains(t, out, "1. A")
				require.Contains(t, out, "2. B")
			},
		},
		{
			name: "connect error surfaces",
			arrange: func() (ts.Runner, ts.Options, *bytes.Buffer, *fakeConnector) {
				fs := fakeSelector{res: sampleResult()}
				fc := &fakeConnector{err: errors.New("boom")}
				r := ts.NewRunner(fs, fc)
				buf := &bytes.Buffer{}
				opts := ts.Options{MatchID: 1, Format: "T20", Season: "2025"}
				return r, opts, buf, fc
			},
			assert: func(t *testing.T, _ *bytes.Buffer, err error, _ *fakeConnector) {
				require.Error(t, err)
				require.ErrorContains(t, err, "db connect failed")
			},
		},
		{
			name: "selection error surfaces",
			arrange: func() (ts.Runner, ts.Options, *bytes.Buffer, *fakeConnector) {
				fs := fakeSelector{err: errors.New("select boom")}
				fc := &fakeConnector{}
				r := ts.NewRunner(fs, fc)
				buf := &bytes.Buffer{}
				opts := ts.Options{MatchID: 1, Format: "T20", Season: "2025", TeamSize: 11}
				return r, opts, buf, fc
			},
			assert: func(t *testing.T, buf *bytes.Buffer, err error, _ *fakeConnector) {
				require.Error(t, err)
				require.ErrorContains(t, err, "select boom")
				require.Empty(t, buf.String())
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			r, opts, buf, fc := tc.arrange()
			// Act
			err := r.Run(context.Background(), opts, buf)
			// Assert
			tc.assert(t, buf, err, fc)
		})
	}
}

// capturingSelector records last call parameters to validate option propagation.
type capturingSelector struct {
	lastOpts selection.Options
}

func (c *capturingSelector) SelectTeam(
	_ context.Context,
	_ int64,
	_ string,
	opts selection.Options,
) (selection.Result, error) {
	c.lastOpts = opts
	return selection.Result{Players: []predictor.PlayerPrediction{{PlayerName: "X", WinningProbability: 0.1}}}, nil
}

func TestOrch_OptionPropagation(t *testing.T) {
	t.Parallel()
	sel := &capturingSelector{}
	fc := &fakeConnector{}
	r := ts.NewRunner(sel, fc)
	buf := &bytes.Buffer{}
	opts := ts.Options{
		MatchID:       1,
		Format:        "T20",
		Season:        "2025",
		TeamSize:      11,
		MinBowlers:    6,
		RequireKeeper: true,
	}
	err := r.Run(context.Background(), opts, buf)
	require.NoError(t, err)
	require.True(t, sel.lastOpts.RequireKeeper)
	require.Equal(t, 6, sel.lastOpts.MinBowlers)
	require.Equal(t, 11, sel.lastOpts.TeamSize)
}
