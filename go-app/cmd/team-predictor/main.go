// Command team-predictor predicts a team lineup for a given match.
// Thin wrapper: parse via internal CLI, wire dependencies, delegate to internal runner.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teampredictor"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/teampredictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/teampredictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/tracking"
)

func main() { os.Exit(run()) }

func run() int {
	fs := flag.NewFlagSet("team-predictor", flag.ContinueOnError)
	opts, err := cli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parse failed", slog.Any("err", err))
		return 2
	}
	logger.SetupFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// DB connect for tracking
	var tracker *tracking.Tracker
	if _, err := db.Connect(ctx); err == nil {
		var tErr error
		tracker, tErr = tracking.Start(ctx, "team-predictor", opts)
		if tErr != nil {
			slog.Warn("tracking start failed", slog.Any("err", tErr))
		}
	}

	service := svc.NewService(nil)
	runner := cmd.NewRunner(service)
	resp, runErr := runner.Run(ctx, opts)
	if runErr != nil {
		slog.Error("team-predictor failed", slog.Any("err", runErr))
		if tracker != nil {
			_ = tracker.Fail(ctx, runErr.Error())
		}
		return 1
	}
	if tracker != nil {
		_ = tracker.Complete(ctx, map[string]int{"players_count": len(resp.Players)})
	}
	// Render simple output (players, one per line)
	for i, p := range resp.Players {
		fmt.Printf("%d. %s\n", i+1, p)
	}
	return 0
}
