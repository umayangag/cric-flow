package teamselect_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/predictor"
	"github.com/umayangag/cric-flow/go-app/internal/selection"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

type fakeConnector struct {
	called int
	err    error
}

func (f *fakeConnector) Connect(_ context.Context) error {
	f.called++
	return f.err
}

type fakeSelector struct {
	calledDB   int
	lastMatch  int64
	lastFormat string
	lastOpts   selection.Options
	res        selection.Result
	err        error
}

func (f *fakeSelector) SelectTeam(
	_ context.Context,
	matchID int64,
	format string,
	opts selection.Options,
) (selection.Result, error) {
	f.calledDB++
	f.lastMatch, f.lastFormat, f.lastOpts = matchID, format, opts
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

func TestRunner_Success(t *testing.T) {
	fs := &fakeSelector{res: sampleResult()}
	fc := &fakeConnector{}
	r := svc.NewRunner(fs, fc)
	buf := &bytes.Buffer{}
	opts := svc.Options{MatchID: 1, Format: "T20", Season: "2025", TeamSize: 11, MinBowlers: 5}
	require.NoError(t, r.Run(context.Background(), opts, buf))
	require.Equal(t, 1, fc.called, "connector called once")
	require.Equal(t, 1, fs.calledDB, "selector DB call")
	require.Equal(t, int64(1), fs.lastMatch)
	require.Equal(t, "T20", fs.lastFormat)
	require.Equal(t, selection.Options{TeamSize: 11, MinBowlers: 5}, fs.lastOpts)
	out := buf.String()
	require.Contains(t, out, "Selected Team (size=2)", "header")
	require.Contains(t, out, "1. A")
	require.Contains(t, out, "2. B")
}

func TestRunner_ConnectError(t *testing.T) {
	fs := &fakeSelector{res: sampleResult()}
	fc := &fakeConnector{err: errors.New("boom")}
	r := svc.NewRunner(fs, fc)
	buf := &bytes.Buffer{}
	err := r.Run(context.Background(), svc.Options{MatchID: 1, Format: "T20", Season: "2025"}, buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "db connect failed")
	require.Equal(t, 0, fs.calledDB, "selection not attempted after connect failure")
}

func TestRunner_SelectError(t *testing.T) {
	fs := &fakeSelector{err: errors.New("select boom")}
	fc := &fakeConnector{}
	r := svc.NewRunner(fs, fc)
	buf := &bytes.Buffer{}
	err := r.Run(context.Background(), svc.Options{MatchID: 1, Format: "T20", Season: "2025"}, buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "select boom")
}
