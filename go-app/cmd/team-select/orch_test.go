package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teamselect"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/teamselect"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
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

func TestOrch_FromDB_Success(t *testing.T) {
	fs := fakeSelector{res: sampleResult()}
	fc := &fakeConnector{}
	r := cmd.NewRunner(fs, fc)
	buf := &bytes.Buffer{}
	opts := cli.Options{FromDB: true, MatchID: 1, Format: "T20", Season: "2025", TeamSize: 11, MinBowlers: 5}
	if err := r.Run(context.Background(), opts, buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fc.called != 1 {
		t.Fatalf("expected Connect called once, got %d", fc.called)
	}
	out := buf.String()
	if !strings.Contains(out, "Selected Team (size=2)") {
		t.Fatalf("output missing header, got: %s", out)
	}
	if !strings.Contains(out, "1. A") || !strings.Contains(out, "2. B") {
		t.Fatalf("output missing players list, got: %s", out)
	}
}

func TestOrch_FromDB_ConnectError(t *testing.T) {
	fs := fakeSelector{res: sampleResult()}
	fc := &fakeConnector{err: errors.New("boom")}
	r := cmd.NewRunner(fs, fc)
	buf := &bytes.Buffer{}
	err := r.Run(context.Background(), cli.Options{FromDB: true, MatchID: 1, Format: "T20", Season: "2025"}, buf)
	if err == nil || !strings.Contains(err.Error(), "db connect failed") {
		t.Fatalf("expected db connect failed error, got %v", err)
	}
}

func TestOrch_FromCSV_Success(t *testing.T) {
	fs := fakeSelector{res: sampleResult()}
	fc := &fakeConnector{}
	r := cmd.NewRunner(fs, fc)
	buf := &bytes.Buffer{}
	opts := cli.Options{
		FromDB:   false,
		PoolPath: "/tmp/pool.csv",
		MatchID:  1,
		Format:   "T20",
		Season:   "2025",
		TeamSize: 11,
	}
	if err := r.Run(context.Background(), opts, buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fc.called != 0 {
		t.Fatalf("Connect should not be called for CSV path, got %d", fc.called)
	}
	out := buf.String()
	if !strings.Contains(out, "Selected Team (size=2)") {
		t.Fatalf("output missing header, got: %s", out)
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
	sel := &capturingSelector{}
	fc := &fakeConnector{}
	r := cmd.NewRunner(sel, fc)
	buf := &bytes.Buffer{}
	opts := cli.Options{
		FromDB:        true,
		MatchID:       1,
		Format:        "T20",
		Season:        "2025",
		TeamSize:      11,
		MinBowlers:    6,
		RequireKeeper: true,
	}
	if err := r.Run(context.Background(), opts, buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sel.lastFromDB || !sel.lastOpts.RequireKeeper || sel.lastOpts.MinBowlers != 6 || sel.lastOpts.TeamSize != 11 {
		t.Fatalf("options not propagated correctly: %#v", sel.lastOpts)
	}
}

func TestOrch_OptionPropagation_CSV(t *testing.T) {
	sel := &capturingSelector{}
	fc := &fakeConnector{}
	r := cmd.NewRunner(sel, fc)
	buf := &bytes.Buffer{}
	opts := cli.Options{
		FromDB:     false,
		PoolPath:   "/tmp/pool.csv",
		MatchID:    1,
		Format:     "T20",
		Season:     "2025",
		TeamSize:   9,
		MinBowlers: 4,
	}
	if err := r.Run(context.Background(), opts, buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel.lastFromDB || sel.lastOpts.RequireKeeper || sel.lastOpts.MinBowlers != 4 || sel.lastOpts.TeamSize != 9 {
		t.Fatalf("options not propagated correctly: %#v", sel.lastOpts)
	}
}
