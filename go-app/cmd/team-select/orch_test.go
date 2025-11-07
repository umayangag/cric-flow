package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

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
