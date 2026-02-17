// Command api starts the HTTP API server for the cricket data service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
	apipkg "github.com/umayangag/cric-info-scrapers/go-app/internal/server"
)

func main() {
	os.Exit(run())
}

func run() int {
	logger.SetupFromEnv()

	if err := config.ValidateForServer(); err != nil {
		slog.Error("config validation failed", slog.Any("err", err))
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		slog.Error("database connection failed", slog.Any("err", err))
		return 1
	}

	// Run migrations on startup (idempotent)
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "/migrations"
	}
	if err := db.RunMigrations(ctx, migrationsDir); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		return 1
	}

	// Initialize long-lived dependencies
	client := mlclient.New()
	server := apipkg.NewApp(client)

	// Build router with dependencies
	r := apipkg.NewRouter(server)

	addr := ":8080"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}
	slog.Info("API listening", slog.String("address", addr))

	// Use http.Server to set timeouts (gosec G114)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server exited", slog.Any("err", err))
		return 1
	}
	return 0
}
