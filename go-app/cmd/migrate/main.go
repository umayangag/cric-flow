// Command migrate runs database schema migrations.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	climig "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/migrate"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/migrate"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
)

func main() {
	// Thin delegator: parse, wire, run.
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	opts, err := climig.ParseArgs(fs, os.Args[1:])
	if err != nil {
		panic(err)
	}

	logger.SetupFromEnv()
	_ = slog.Default()

	r := cmd.Runner{Migrate: db.RunMigrations}
	if err := r.Run(context.Background(), opts.Dir); err != nil {
		panic(err)
	}
}
