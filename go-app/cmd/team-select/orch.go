package main

import (
	"context"
	"fmt"
	"io"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
)

// selector provides an injectable seam over selection package functions for testing.
// realSelector calls through to the selection package.
type selector interface {
	SelectTeam(ctx context.Context, matchID int64, format, season string, opts selection.Options) (selection.Result, error)
	SelectTeamFromCSV(ctx context.Context, pool string, matchID int64, format, season string, opts selection.Options) (selection.Result, error)
}

type realSelector struct{}

func (realSelector) SelectTeam(ctx context.Context, matchID int64, format, season string, opts selection.Options) (selection.Result, error) {
	return selection.SelectTeam(ctx, matchID, format, season, opts)
}

func (realSelector) SelectTeamFromCSV(ctx context.Context, pool string, matchID int64, format, season string, opts selection.Options) (selection.Result, error) {
	return selection.SelectTeamFromCSV(ctx, pool, matchID, format, season, opts)
}

// runSelection orchestrates DB connectivity (if needed), selection execution, and output printing.
// It is pure w.r.t. external systems besides the provided connector and writer, enabling offline tests.
func runSelection(ctx context.Context, connector db.Connector, sel selector, opts options, w io.Writer) error {
	// Ensure DB connection available when using DB mode
	if opts.fromDB {
		if err := connector.Connect(ctx); err != nil {
			return fmt.Errorf("db connect failed: %w", err)
		}
	}

	selOpts := selection.Options{TeamSize: opts.teamSize, MinBowlers: opts.minBowlers, RequireKeeper: opts.requireKeeper}
	var (
		res selection.Result
		err error
	)
	if opts.fromDB {
		res, err = sel.SelectTeam(ctx, opts.matchID, opts.formatCode, opts.seasonName, selOpts)
	} else {
		res, err = sel.SelectTeamFromCSV(ctx, opts.poolPath, opts.matchID, opts.formatCode, opts.seasonName, selOpts)
	}
	if err != nil {
		return fmt.Errorf("selection failed: %w", err)
	}

	// Print the selected team (mirrors main.go formatting)
	_, _ = fmt.Fprintf(w, "Selected Team (size=%d) — Team Win Prob: %.4f\n", len(res.Players), res.TeamWinProbability)
	_, _ = fmt.Fprintln(w, "-----------------------------------------------------------")
	for i, p := range res.Players {
		_, _ = fmt.Fprintf(w, "%2d. %-24s  win=%.4f  bat_pos=%.0f  runs=%.1f  wkts=%.1f\n",
			i+1, p.PlayerName, p.WinningProbability, p.BattingPosition, p.RunsScored, p.WicketsTaken)
	}
	return nil
}
