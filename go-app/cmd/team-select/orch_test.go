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
// They ensure the thin wiring prints the expected output and respects DB vs CSV paths.

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
	_, _ string,
	_ selection.Options,
) (selection.Result, error) {
	return f.res, f.err
}

func (f fakeSelector) SelectTeamFromCSV(
	_ context.Context,
	_ string,
	_ int64,
	_, _ string,
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

	cases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "FromDB success prints team and connects once",
			arrange: func() (ts.Runner, ts.Options, *bytes.Buffer, *fakeConnector) {
				fs := fakeSelector{res: sampleResult()}
				fc := &fakeConnector{}
				r := ts.NewRunner(fs, fc)
				buf := &bytes.Buffer{}
				opts := ts.Options{
					FromDB:     true,
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
			name: "FromDB connect error surfaces",
			arrange: func() (ts.Runner, ts.Options, *bytes.Buffer, *fakeConnector) {
				fs := fakeSelector{res: sampleResult()}
				fc := &fakeConnector{err: errors.New("boom")}
				r := ts.NewRunner(fs, fc)
				buf := &bytes.Buffer{}
				opts := ts.Options{FromDB: true, MatchID: 1, Format: "T20", Season: "2025"}
				return r, opts, buf, fc
			},
			assert: func(t *testing.T, _ *bytes.Buffer, err error, _ *fakeConnector) {
				require.Error(t, err)
				require.ErrorContains(t, err, "db connect failed")
			},
		},
		{
			name: "FromCSV success prints team and does not connect",
			arrange: func() (ts.Runner, ts.Options, *bytes.Buffer, *fakeConnector) {
				fs := fakeSelector{res: sampleResult()}
				fc := &fakeConnector{}
				r := ts.NewRunner(fs, fc)
				buf := &bytes.Buffer{}
				opts := ts.Options{
					FromDB:   false,
					PoolPath: "/tmp/pool.csv",
					MatchID:  1,
					Format:   "T20",
					Season:   "2025",
					TeamSize: 11,
				}
				return r, opts, buf, fc
			},
			assert: func(t *testing.T, buf *bytes.Buffer, err error, fc *fakeConnector) {
				require.NoError(t, err)
				require.Equal(t, 0, fc.called)
				out := buf.String()
				require.Contains(t, out, "Selected Team (size=2)")
			},
		},
	}

	for _, tc := range cases {
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
	lastFromDB bool
	lastOpts   selection.Options
}

func (c *capturingSelector) SelectTeam(
	_ context.Context,
	_ int64,
	_, _ string,
	opts selection.Options,
) (selection.Result, error) {
	c.lastFromDB = true
	c.lastOpts = opts
	return selection.Result{Players: []predictor.PlayerPrediction{{PlayerName: "X", WinningProbability: 0.1}}}, nil
}

func (c *capturingSelector) SelectTeamFromCSV(
	_ context.Context,
	_ string,
	_ int64,
	_, _ string,
	opts selection.Options,
) (selection.Result, error) {
	c.lastFromDB = false
	c.lastOpts = opts
	return selection.Result{Players: []predictor.PlayerPrediction{{PlayerName: "Y", WinningProbability: 0.2}}}, nil
}

func TestOrch_OptionPropagation_DB(t *testing.T) {
	t.Parallel()
	sel := &capturingSelector{}
	fc := &fakeConnector{}
	r := ts.NewRunner(sel, fc)
	buf := &bytes.Buffer{}
	opts := ts.Options{
		FromDB:        true,
		MatchID:       1,
		Format:        "T20",
		Season:        "2025",
		TeamSize:      11,
		MinBowlers:    6,
		RequireKeeper: true,
	}
	err := r.Run(context.Background(), opts, buf)
	require.NoError(t, err)
	require.True(t, sel.lastFromDB)
	require.True(t, sel.lastOpts.RequireKeeper)
	require.Equal(t, 6, sel.lastOpts.MinBowlers)
	require.Equal(t, 11, sel.lastOpts.TeamSize)
}

func TestOrch_OptionPropagation_CSV(t *testing.T) {
	t.Parallel()
	sel := &capturingSelector{}
	fc := &fakeConnector{}
	r := ts.NewRunner(sel, fc)
	buf := &bytes.Buffer{}
	opts := ts.Options{
		FromDB:     false,
		PoolPath:   "/tmp/pool.csv",
		MatchID:    1,
		Format:     "T20",
		Season:     "2025",
		TeamSize:   9,
		MinBowlers: 4,
	}
	err := r.Run(context.Background(), opts, buf)
	require.NoError(t, err)
	require.False(t, sel.lastFromDB)
	require.False(t, sel.lastOpts.RequireKeeper)
	require.Equal(t, 4, sel.lastOpts.MinBowlers)
	require.Equal(t, 9, sel.lastOpts.TeamSize)
}
