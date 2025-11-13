// Command team-select selects a team for a given match by applying simple
// constraints to a candidate player pool loaded from DB or CSV.
// Thin wrapper: parse via internal CLI, wire deps, delegate to internal runner.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teamselect"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/teamselect"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
)

func main() { os.Exit(run()) }

func run() int {
	fs := flag.NewFlagSet("team-select", flag.ContinueOnError)
	opts, err := cli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parse failed", slog.Any("err", err))
		return 2
	}

	logger.SetupFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Build runner with selector adapter and DB connector (only used when FromDB)
	runner := cmd.NewRunner(cmd.NewSelectionAdapter(), db.RealConnector{})
	if runErr := runner.Run(ctx, opts, os.Stdout); runErr != nil {
		slog.Error("team-select failed", slog.Any("err", runErr))
		return 1
	}
	return 0
}
