package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
)

type failingConnector struct{}

func (failingConnector) Connect(_ context.Context) error { return errors.New("connect failed") }

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
