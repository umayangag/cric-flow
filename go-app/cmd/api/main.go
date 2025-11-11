// Command api starts the HTTP API server for the cricket data service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
)

func main() {
	logger.SetupFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("database connection failed", slog.Any("err", err))
		os.Exit(1)
	}

	// Run migrations on startup (idempotent)
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "/migrations"
	}
	if err := db.RunMigrations(ctx, migrationsDir); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		os.Exit(1)
	}

	// Initialize long-lived dependencies
	client := mlclient.New()
	server := newApp(client)

	// Build router with dependencies
	r := newRouter(server)

	addr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}
	slog.Info("API listening", slog.String("address", addr))
	if err := http.ListenAndServe(addr, r); err != nil {
		slog.Error("server exited", slog.Any("err", err))
		os.Exit(1)
	}
}
