package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	dbmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/db/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
)

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
