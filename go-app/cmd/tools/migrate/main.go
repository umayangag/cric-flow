// Command migrate runs database schema migrations.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
)

func main() {
	var dir string
	flag.StringVar(&dir, "dir", "migrations", "directory with .sql migration files")
	flag.Parse()

	logger.SetupFromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		os.Exit(1)
	}
	if err := db.RunMigrations(ctx, dir); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		os.Exit(1)
	}
	slog.Info("migrations applied successfully", slog.String("dir", dir))
}
