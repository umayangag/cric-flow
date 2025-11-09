// Command team-select runs the end-to-end team selection pipeline analogous to src/team_selection/select_pool.py
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/selection"
)

func main() {
	logger.SetupFromEnv()
	cfg := config.Load()

	opts, err := parseFlags(os.Args[1:], cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Ensure DB connection available when using DB mode
 if opts.fromDB {
		if _, err := db.Connect(ctx); err != nil {
			slog.Error("db connect failed", slog.Any("err", err))
			os.Exit(1)
		}
	}

	selOpts := selection.Options{
		TeamSize:      opts.teamSize,
		MinBowlers:    opts.minBowlers,
		RequireKeeper: opts.requireKeeper,
	}
	var res selection.Result
	if opts.fromDB {
		res, err = selection.SelectTeam(ctx, opts.matchID, opts.formatCode, opts.seasonName, selOpts)
	} else {
		res, err = selection.SelectTeamFromCSV(ctx, opts.poolPath, opts.matchID, opts.formatCode, opts.seasonName, selOpts)
	}
 if err != nil {
		slog.Error("selection failed", slog.Any("err", err))
		os.Exit(1)
	}

	fmt.Printf("Selected Team (size=%d) — Team Win Prob: %.4f\n", len(res.Players), res.TeamWinProbability)
	fmt.Println("-----------------------------------------------------------")
	for i, p := range res.Players {
		fmt.Printf("%2d. %-24s  win=%.4f  bat_pos=%.0f  runs=%.1f  wkts=%.1f\n",
			i+1, p.PlayerName, p.WinningProbability, p.BattingPosition, p.RunsScored, p.WicketsTaken)
	}
}
