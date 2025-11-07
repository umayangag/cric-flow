package main

import (
	"context"
	"fmt"
	"io"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
)

// TeamSelector abstracts selection operations to enable offline tests.
// The real implementation is the selection package via its functions.
type TeamSelector interface {
	SelectTeam(
		ctx context.Context,
		matchID int64,
		format, season string,
		opts selection.Options,
	) (selection.Result, error)
	SelectTeamFromCSV(
		ctx context.Context,
		poolPath string,
		matchID int64,
		format, season string,
		opts selection.Options,
	) (selection.Result, error)
}

// realSelector delegates to selection package functions.
type realSelector struct{}

func (realSelector) SelectTeam(
	ctx context.Context,
	matchID int64,
	format, season string,
	opts selection.Options,
) (selection.Result, error) {
	return selection.SelectTeam(ctx, matchID, format, season, opts)
}

func (realSelector) SelectTeamFromCSV(
	ctx context.Context,
	poolPath string,
	matchID int64,
	format, season string,
	opts selection.Options,
) (selection.Result, error) {
	return selection.SelectTeamFromCSV(ctx, poolPath, matchID, format, season, opts)
}

// runSelection performs selection using either DB or CSV according to opts.fromDB, writing a
// deterministic summary to w. It accepts a db.Connector and TeamSelector to allow offline tests.
func runSelection(ctx context.Context, connector db.Connector, sel TeamSelector, w io.Writer, opts options) error {
	if opts.fromDB {
		if err := connector.Connect(ctx); err != nil {
			return fmt.Errorf("db connect failed: %w", err)
		}
		res, err := sel.SelectTeam(ctx, opts.matchID, opts.formatCode, opts.seasonName, selection.Options{
			TeamSize:      opts.teamSize,
			MinBowlers:    opts.minBowlers,
			RequireKeeper: opts.requireKeeper,
		})
		if err != nil {
			return err
		}
		printResult(w, res)
		return nil
	}
	res, err := sel.SelectTeamFromCSV(
		ctx,
		opts.poolPath,
		opts.matchID,
		opts.formatCode,
		opts.seasonName,
		selection.Options{
			TeamSize:      opts.teamSize,
			MinBowlers:    opts.minBowlers,
			RequireKeeper: opts.requireKeeper,
		},
	)
	if err != nil {
		return err
	}
	printResult(w, res)
	return nil
}

func printResult(w io.Writer, res selection.Result) {
	fmt.Fprintf(w, "Selected Team (size=%d) — Team Win Prob: %.4f\n", len(res.Players), res.TeamWinProbability)
	for i, p := range res.Players {
		fmt.Fprintf(w, "%2d. %s %.4f\n", i+1, p.PlayerName, p.WinningProbability)
	}
}
