// Command migrate runs database schema migrations.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	migsvc "github.com/umayangag/cric-flow/go-app/internal/services/migrate"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/logger"
)

func main() {
	// Thin delegator: parse, wire, run.
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	opts, err := migsvc.ParseArgs(fs, os.Args[1:])
	if err != nil {
		panic(err)
	}

	logger.SetupFromEnv()
	_ = slog.Default()

	r := migsvc.Runner{Migrate: db.RunMigrations}
	if err := r.Run(context.Background(), opts.Dir); err != nil {
		panic(err)
	}
}
