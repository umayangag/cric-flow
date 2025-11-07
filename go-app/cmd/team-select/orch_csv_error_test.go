package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
)

type csvErrorSelector struct{}

func (csvErrorSelector) SelectTeam(
	ctx context.Context,
	matchID int64,
	format, season string,
	opts selection.Options,
) (selection.Result, error) {
	return selection.Result{}, nil
}

func (csvErrorSelector) SelectTeamFromCSV(
	ctx context.Context,
	poolPath string,
	matchID int64,
	format, season string,
	opts selection.Options,
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
