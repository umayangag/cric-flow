//go:build never

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	dbmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/db/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
)

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

func TestRunSelection_FromDB_Success(t *testing.T) {
	fc := &fakeConnector{}
	fs := fakeSelector{res: sampleResult()}
	w := &bytes.Buffer{}
	opts := options{fromDB: true, matchID: 1, formatCode: "T20", seasonName: "2025", teamSize: 11, minBowlers: 5}

	if err := runSelection(context.Background(), fc, fs, w, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fc.called != 1 {
		t.Fatalf("expected Connect called once, got %d", fc.called)
	}
	out := w.String()
	if !strings.Contains(out, "Selected Team (size=2)") { // two players in sample
		t.Fatalf("output missing header, got: %s", out)
	}
	if !strings.Contains(out, "1. A") || !strings.Contains(out, "2. B") {
		t.Fatalf("output missing players list, got: %s", out)
	}
}

func TestRunSelection_FromDB_ConnectError(t *testing.T) {
	fc := &fakeConnector{err: errors.New("boom")}
	fs := fakeSelector{res: sampleResult()}
	w := &bytes.Buffer{}
	opts := options{fromDB: true, matchID: 1, formatCode: "T20", seasonName: "2025"}

	err := runSelection(context.Background(), fc, fs, w, opts)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "db connect failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunSelection_FromCSV_Success(t *testing.T) {
	fc := &fakeConnector{}
	fs := fakeSelector{res: sampleResult()}
	w := &bytes.Buffer{}
	opts := options{fromDB: false, poolPath: "pool.csv", matchID: 1, formatCode: "T20", seasonName: "2025"}

	if err := runSelection(context.Background(), fc, fs, w, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fc.called != 0 {
		t.Fatalf("Connect should not be called for CSV path, got %d", fc.called)
	}
	out := w.String()
	if !strings.Contains(out, "Selected Team (size=2)") {
		t.Fatalf("output missing header, got: %s", out)
	}
}

// Consolidated additional tests and helpers from orch_error_test.go, orch_csv_error_test.go, and orch_options_test.go

// failingConnector is used to simulate DB connection failures.
type failingConnector struct{}

func (failingConnector) Connect(_ context.Context) error { return errors.New("connect failed") }

// noOpSelector implements selection.Selector with no-op responses.
type noOpSelector struct{}

func (noOpSelector) SelectTeam(
	_ context.Context,
	_ int64,
	_, _ string,
	_ selection.Options,
) (selection.Result, error) {
	return selection.Result{}, nil
}

func (noOpSelector) SelectTeamFromCSV(
	_ context.Context,
	_ string,
	_ int64,
	_, _ string,
	_ selection.Options,
) (selection.Result, error) {
	return selection.Result{}, nil
}

func TestRunSelection_DBConnectError(t *testing.T) {
	ctx := context.Background()
	conn := failingConnector{}
	sel := noOpSelector{}
	var buf bytes.Buffer
	opts := options{matchID: 1, formatCode: "T20", seasonName: "2025", fromDB: true, teamSize: 11, minBowlers: 5}
	if err := runSelection(ctx, conn, sel, &buf, opts); err == nil {
		t.Fatalf("expected db connect error")
	}
}

// csvErrorSelector simulates a CSV selection failure.
type csvErrorSelector struct{}

func (csvErrorSelector) SelectTeam(
	_ context.Context,
	_ int64,
	_, _ string,
	_ selection.Options,
) (selection.Result, error) {
	return selection.Result{}, nil
}

func (csvErrorSelector) SelectTeamFromCSV(
	_ context.Context,
	_ string,
	_ int64,
	_, _ string,
	_ selection.Options,
) (selection.Result, error) {
	return selection.Result{}, errors.New("csv failed")
}

func TestRunSelection_CSVSelectorError(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	conn := failingConnector{}
	sel := csvErrorSelector{}
	opts := options{
		matchID:    9,
		formatCode: "T20",
		seasonName: "2025",
		fromDB:     false,
		poolPath:   "/tmp/pool.csv",
		teamSize:   11,
		minBowlers: 5,
	}
	if err := runSelection(ctx, conn, sel, &buf, opts); err == nil {
		t.Fatalf("expected csv selector error")
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

func TestRunSelection_PassesRequireKeeperAndMinBowlers_DB(t *testing.T) {
	ctx := context.Background()
	m := &dbmocks.Connector{}
	m.On("Connect", mock.Anything).Return(nil)
	sel := &capturingSelector{}
	var buf bytes.Buffer
	opts := options{
		matchID:       1,
		formatCode:    "T20",
		seasonName:    "2025",
		fromDB:        true,
		teamSize:      11,
		minBowlers:    6,
		requireKeeper: true,
	}
	if err := runSelection(ctx, m, sel, &buf, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sel.lastFromDB {
		t.Fatalf("expected DB path")
	}
	if !sel.lastOpts.RequireKeeper || sel.lastOpts.MinBowlers != 6 || sel.lastOpts.TeamSize != 11 {
		t.Fatalf("options not propagated correctly: %#v", sel.lastOpts)
	}
}

func TestRunSelection_PassesRequireKeeperAndMinBowlers_CSV(t *testing.T) {
	ctx := context.Background()
	m := &dbmocks.Connector{}
	// Not expected to be called for CSV path; leave default zero behavior.
	sel := &capturingSelector{}
	var buf bytes.Buffer
	opts := options{
		matchID:       1,
		formatCode:    "T20",
		seasonName:    "2025",
		fromDB:        false,
		poolPath:      "/tmp/pool.csv",
		teamSize:      9,
		minBowlers:    4,
		requireKeeper: false,
	}
	if err := runSelection(ctx, m, sel, &buf, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel.lastFromDB {
		t.Fatalf("expected CSV path")
	}
	if sel.lastOpts.RequireKeeper || sel.lastOpts.MinBowlers != 4 || sel.lastOpts.TeamSize != 9 {
		t.Fatalf("options not propagated correctly: %#v", sel.lastOpts)
	}
}
