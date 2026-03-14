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
	calledCSV  int
	lastPool   string
	lastMatch  int64
	lastFormat string
	lastSeason string
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

func (f *fakeSelector) SelectTeamFromCSV(
	_ context.Context,
	poolPath string,
	matchID int64,
	format, season string,
	opts selection.Options,
) (selection.Result, error) {
	f.calledCSV++
	f.lastPool, f.lastMatch, f.lastFormat, f.lastSeason, f.lastOpts = poolPath, matchID, format, season, opts
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

func TestRunner_FromDB_Success(t *testing.T) {
	fs := &fakeSelector{res: sampleResult()}
	fc := &fakeConnector{}
	r := svc.NewRunner(fs, fc)
	buf := &bytes.Buffer{}
	opts := svc.Options{FromDB: true, MatchID: 1, Format: "T20", Season: "2025", TeamSize: 11, MinBowlers: 5}
	require.NoError(t, r.Run(context.Background(), opts, buf))
	require.Equal(t, 1, fc.called, "connector called once")
	require.Equal(t, 1, fs.calledDB, "selector DB call")
	require.Equal(t, 0, fs.calledCSV, "selector CSV call")
	out := buf.String()
	require.Contains(t, out, "Selected Team (size=2)", "header")
	require.Contains(t, out, "1. A")
	require.Contains(t, out, "2. B")
}

func TestRunner_FromDB_ConnectError(t *testing.T) {
	fs := &fakeSelector{res: sampleResult()}
	fc := &fakeConnector{err: errors.New("boom")}
	r := svc.NewRunner(fs, fc)
	buf := &bytes.Buffer{}
	err := r.Run(context.Background(), svc.Options{FromDB: true, MatchID: 1, Format: "T20", Season: "2025"}, buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "db connect failed")
}

func TestRunner_FromCSV_Success(t *testing.T) {
	fs := &fakeSelector{res: sampleResult()}
	fc := &fakeConnector{}
	r := svc.NewRunner(fs, fc)
	buf := &bytes.Buffer{}
	opts := svc.Options{
		FromDB:   false,
		PoolPath: "/tmp/pool.csv",
		MatchID:  1,
		Format:   "ODI",
		Season:   "2019",
		TeamSize: 11,
	}
	require.NoError(t, r.Run(context.Background(), opts, buf))
	require.Equal(t, 0, fc.called, "connector not called for CSV")
	require.Equal(t, 1, fs.calledCSV)
	require.Equal(t, 0, fs.calledDB)
	require.Equal(t, "/tmp/pool.csv", fs.lastPool)
	require.Equal(t, int64(1), fs.lastMatch)
	require.Equal(t, "ODI", fs.lastFormat)
	require.Equal(t, "2019", fs.lastSeason)
}
