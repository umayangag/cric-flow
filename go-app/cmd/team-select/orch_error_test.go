package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
)

type failingConnector struct{}

func (failingConnector) Connect(ctx context.Context) error { return errors.New("connect failed") }

type noOpSelector struct{}

func (noOpSelector) SelectTeam(ctx context.Context, matchID int64, format, season string, opts selection.Options) (selection.Result, error) {
	return selection.Result{}, nil
}

func (noOpSelector) SelectTeamFromCSV(ctx context.Context, poolPath string, matchID int64, format, season string, opts selection.Options) (selection.Result, error) {
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
