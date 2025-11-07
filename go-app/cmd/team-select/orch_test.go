package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	dbfake "github.com/umayangag/cric-info-scrapers/go-app/internal/db/fake"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
)

type selFake struct {
	res selection.Result
	err error
	usedCSV bool
}

func (s *selFake) SelectTeam(ctx context.Context, matchID int64, format, season string, opts selection.Options) (selection.Result, error) {
	s.usedCSV = false
	return s.res, s.err
}

func (s *selFake) SelectTeamFromCSV(ctx context.Context, pool string, matchID int64, format, season string, opts selection.Options) (selection.Result, error) {
	s.usedCSV = true
	return s.res, s.err
}

func TestRunSelection_DBMode_Success(t *testing.T) {
	connector := dbfake.Connector{}
	sel := &selFake{res: selection.Result{
		Players: []predictor.PlayerPrediction{
			{PlayerName: "A", WinningProbability: 0.9},
			{PlayerName: "B", WinningProbability: 0.8},
		},
		TeamWinProbability: 0.75,
	}}
	opts := options{matchID: 1, formatCode: "T20", seasonName: "2025", teamSize: 2, minBowlers: 1, requireKeeper: false, fromDB: true}
	var buf bytes.Buffer
	if err := runSelection(context.Background(), connector, sel, opts, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !sel.usedCSV && !(contains(out, "Selected Team") && contains(out, "A") && contains(out, "B")) {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRunSelection_CSVMode_Success(t *testing.T) {
	connector := dbfake.Connector{} // not used in CSV mode
	sel := &selFake{res: selection.Result{Players: []predictor.PlayerPrediction{{PlayerName: "X"}}, TeamWinProbability: 0.5}}
	opts := options{matchID: 1, formatCode: "T20", seasonName: "2025", poolPath: "pool.csv", fromDB: false}
	var buf bytes.Buffer
	if err := runSelection(context.Background(), connector, sel, opts, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sel.usedCSV {
		t.Fatalf("expected CSV selector path to be used")
	}
	if !contains(buf.String(), "X") {
		t.Fatalf("expected output to reference player 'X': %q", buf.String())
	}
}

func TestRunSelection_SelectionError(t *testing.T) {
	connector := dbfake.Connector{}
	sel := &selFake{err: errors.New("boom")}
	opts := options{matchID: 1, formatCode: "T20", seasonName: "2025", fromDB: true}
	var buf bytes.Buffer
	if err := runSelection(context.Background(), connector, sel, opts, &buf); err == nil {
		t.Fatalf("expected error from selection")
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }
