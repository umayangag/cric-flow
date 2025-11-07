package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	dbfake "github.com/umayangag/cric-info-scrapers/go-app/internal/db/fake"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
)

type fakeSelector struct {
	res selection.Result
	err error
}

func (f fakeSelector) SelectTeam(ctx context.Context, matchID int64, format, season string, opts selection.Options) (selection.Result, error) {
	return f.res, f.err
}

func (f fakeSelector) SelectTeamFromCSV(ctx context.Context, poolPath string, matchID int64, format, season string, opts selection.Options) (selection.Result, error) {
	return f.res, f.err
}

func TestRunSelection_DBMode_Success(t *testing.T) {
	ctx := context.Background()
	connector := dbfake.Connector{}
	players := []predictor.PlayerPrediction{{PlayerName: "Alice", WinningProbability: 0.9}}
	res := selection.Result{Players: players, TeamWinProbability: 0.77}
	sel := fakeSelector{res: res}

	opts := options{
		matchID:       1,
		formatCode:    "T20",
		seasonName:    "2025",
		poolPath:      "unused.csv",
		teamSize:      11,
		minBowlers:    5,
		requireKeeper: false,
		fromDB:        true,
	}
	var buf bytes.Buffer
	if err := runSelection(ctx, connector, sel, &buf, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Selected Team (size=1)") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "Alice 0.9000") {
		t.Fatalf("expected player line with prob, got: %s", out)
	}
}

func TestRunSelection_CSVMode_Success(t *testing.T) {
	ctx := context.Background()
	connector := dbfake.Connector{}
	players := []predictor.PlayerPrediction{{PlayerName: "Bob", WinningProbability: 0.5}}
	res := selection.Result{Players: players, TeamWinProbability: 0.55}
	sel := fakeSelector{res: res}

	opts := options{
		matchID:       2,
		formatCode:    "ODI",
		seasonName:    "2019",
		poolPath:      "/tmp/pool.csv",
		teamSize:      9,
		minBowlers:    4,
		requireKeeper: true,
		fromDB:        false,
	}
	var buf bytes.Buffer
	if err := runSelection(ctx, connector, sel, &buf, opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Selected Team (size=1)") || !strings.Contains(out, "Bob 0.5000") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunSelection_SelectorError(t *testing.T) {
	ctx := context.Background()
	connector := dbfake.Connector{}
	sel := fakeSelector{err: errors.New("boom")}

	opts := options{
		matchID:    3,
		formatCode: "T20",
		seasonName: "2024",
		fromDB:     true,
		teamSize:   11,
	}
	var buf bytes.Buffer
	if err := runSelection(ctx, connector, sel, &buf, opts); err == nil {
		t.Fatalf("expected error from selector")
	}
}
