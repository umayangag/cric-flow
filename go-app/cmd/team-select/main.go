// Command team-select selects a team for a given match by applying simple
// constraints to a candidate player pool loaded from DB or CSV.
// Thin wrapper: parse via internal CLI, wire deps, delegate to internal runner.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teamselect"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/teamselect"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	ts "github.com/umayangag/cric-info-scrapers/go-app/internal/services/teamselect"
)

// repo adapter placeholder; returns error if used (to avoid DB coupling here).
// For real DB-backed loads, implement an adapter under internal/adapters/db/teamselectrepo.
type notImplementedRepo struct{}

func (notImplementedRepo) LoadPool(context.Context, int64, string, string) ([]db.PoolPlayer, error) {
	return nil, fmt.Errorf("team-select DB adapter not implemented")
}

func main() {
	fs := flag.NewFlagSet("team-select", flag.ContinueOnError)
	opts, err := cli.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parse failed", slog.Any("err", err))
		os.Exit(2)
	}

	logger.SetupFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Load pool
	var pool []ts.Player
	if opts.FromDB {
		if _, err := db.Connect(ctx); err != nil {
			slog.Error("db connect failed", slog.Any("err", err))
			os.Exit(1)
		}
		repo := notImplementedRepo{}
		pps, lerr := ts.LoadFromDB(ctx, repo, opts.MatchID, opts.Format, opts.Season)
		if lerr != nil {
			slog.Error("load pool from DB failed", slog.Any("err", lerr))
			os.Exit(1)
		}
		pool = pps
	} else {
		fh, oerr := os.Open(opts.PoolCSV)
		if oerr != nil {
			slog.Error("open pool csv failed", slog.Any("err", oerr))
			os.Exit(1)
		}
		defer func() { _ = fh.Close() }()
		pps, perr := ts.LoadFromCSV(fh)
		if perr != nil {
			slog.Error("parse pool csv failed", slog.Any("err", perr))
			os.Exit(1)
		}
		pool = pps
	}

	runner := cmd.NewRunner()
	team, runErr := runner.Run(ctx, opts, pool)
	if runErr != nil {
		slog.Error("team-select failed", slog.Any("err", runErr))
		os.Exit(1)
	}

	for i, p := range team {
		fmt.Printf("%d. %s\n", i+1, p.Name)
	}
}
