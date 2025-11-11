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

	mldummy "github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/mlclient/dummy"
	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teampredictor"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/teampredictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/teampredictor"
)

func main() {
	fs := flag.NewFlagSet("team-predictor", flag.ContinueOnError)
	opts, err := cli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parse failed", slog.Any("err", err))
		os.Exit(2)
	}
	logger.SetupFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ml := mldummy.New()
	service := svc.NewService(ml)
	runner := cmd.NewRunner(service)
	resp, runErr := runner.Run(ctx, opts)
	if runErr != nil {
		slog.Error("team-predictor failed", slog.Any("err", runErr))
		os.Exit(1)
	}
	// Render simple output (players, one per line)
	for i, p := range resp.Players {
		fmt.Printf("%d. %s\n", i+1, p)
	}
}
