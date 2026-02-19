// Command team-predictor predicts a team lineup for a given match.
// Thin wrapper: parse via internal CLI, wire dependencies, delegate to internal runner.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	cli "github.com/umayangag/cric-flow/go-app/internal/cli/teampredictor"
	tpcmd "github.com/umayangag/cric-flow/go-app/internal/commands/teampredictor"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/logger"
	"github.com/umayangag/cric-flow/go-app/internal/mlclient"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/teampredictor"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
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

	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Minute)
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

	var runErr error
	var resp mlclient.PredictResponse
	defer func() {
		var meta map[string]int
		if len(resp.Players) > 0 {
			meta = map[string]int{"players_count": len(resp.Players)}
		}
		tracker.CaptureExit(ctx, &runErr, meta)
	}()

	service := svc.NewService(nil)
	runner := tpcmd.NewRunner(service)
	resp, runErr = runner.Run(ctx, opts)
	if runErr != nil {
		slog.Error("team-predictor failed", slog.Any("err", runErr))
		return 1
	}
	// Render simple output (players, one per line)
	for i, p := range resp.Players {
		fmt.Printf("%d. %s\n", i+1, p)
	}
	return 0
}
