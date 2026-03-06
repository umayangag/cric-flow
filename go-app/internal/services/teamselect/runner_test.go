package teamselect_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

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
	if err := r.Run(context.Background(), opts, buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fc.called != 1 {
		t.Fatalf("expected connector called once, got %d", fc.called)
	}
	if fs.calledDB != 1 || fs.calledCSV != 0 {
		t.Fatalf("selector calls mismatch: db=%d csv=%d", fs.calledDB, fs.calledCSV)
	}
	out := buf.String()
	if !strings.Contains(out, "Selected Team (size=2)") {
		t.Fatalf("missing header, got: %s", out)
	}
	if !strings.Contains(out, "1. A") || !strings.Contains(out, "2. B") {
		t.Fatalf("missing players, got: %s", out)
	}
}

func TestRunner_FromDB_ConnectError(t *testing.T) {
	fs := &fakeSelector{res: sampleResult()}
	fc := &fakeConnector{err: errors.New("boom")}
	r := svc.NewRunner(fs, fc)
	buf := &bytes.Buffer{}
	err := r.Run(context.Background(), svc.Options{FromDB: true, MatchID: 1, Format: "T20", Season: "2025"}, buf)
	if err == nil || !strings.Contains(err.Error(), "db connect failed") {
		t.Fatalf("expected db connect failed error, got %v", err)
	}
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
	if err := r.Run(context.Background(), opts, buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fc.called != 0 {
		t.Fatalf("connector should not be called for CSV, got %d", fc.called)
	}
	if fs.calledCSV != 1 || fs.calledDB != 0 {
		t.Fatalf("selector calls mismatch: db=%d csv=%d", fs.calledDB, fs.calledCSV)
	}
	if fs.lastPool != "/tmp/pool.csv" || fs.lastMatch != 1 || fs.lastFormat != "ODI" || fs.lastSeason != "2019" {
		t.Fatalf("selector args mismatch: %+v", fs)
	}
}
