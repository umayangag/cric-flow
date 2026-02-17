// Command api starts the HTTP API server for the cricket data service.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/logger"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
	apipkg "github.com/umayangag/cric-info-scrapers/go-app/internal/server"
)

const shutdownTimeout = 25 * time.Second

func main() {
	os.Exit(run())
}

func run() int {
	// Ensure any panic is logged with stack trace before exit (e.g. precompute or init crash).
	defer func() {
		if v := recover(); v != nil {
			slog.Error("api panic (crash)",
				slog.String("panic", fmt.Sprint(v)),
				slog.String("stack", string(debug.Stack())),
			)
			os.Exit(1)
		}
	}()

	logger.SetupFromEnv()

	// Optional memory stats: log at startup and periodically to help diagnose OOM (set MEM_STATS_INTERVAL e.g. 5m).
	logMemStatsOnce()
	if d := memStatsInterval(); d > 0 {
		go logMemStatsLoop(d)
	}

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

	// Graceful shutdown: on SIGTERM/SIGINT, shut down server and close DB so logs are flushed.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		} else {
			serverErr <- nil
		}
	}()

	select {
	case sig := <-quit:
		slog.Info("shutdown requested", slog.String("signal", sig.String()))
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown failed (timeout or error)", slog.Any("err", err))
			db.Close()
			return 1
		}
		slog.Info("server shutdown complete")
		db.Close()
		return 0
	case err := <-serverErr:
		if err != nil {
			slog.Error("server exited with error", slog.Any("err", err))
			db.Close()
			return 1
		}
		return 0
	}
}

// memStatsInterval returns MEM_STATS_INTERVAL (e.g. 5m) for periodic memory logging; 0 disables.
func memStatsInterval() time.Duration {
	s := os.Getenv("MEM_STATS_INTERVAL")
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

func logMemStatsOnce() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	slog.Info("memory stats",
		slog.Uint64("heap_alloc_mb", m.Alloc/(1024*1024)),
		slog.Uint64("heap_sys_mb", m.HeapSys/(1024*1024)),
		slog.Uint64("heap_inuse_mb", m.HeapInuse/(1024*1024)),
		slog.Uint64("sys_mb", m.Sys/(1024*1024)),
		slog.Uint64("num_gc", uint64(m.NumGC)),
	)
}

func logMemStatsLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		slog.Info("memory stats (periodic)",
			slog.Uint64("heap_alloc_mb", m.Alloc/(1024*1024)),
			slog.Uint64("heap_sys_mb", m.HeapSys/(1024*1024)),
			slog.Uint64("heap_inuse_mb", m.HeapInuse/(1024*1024)),
			slog.Uint64("sys_mb", m.Sys/(1024*1024)),
			slog.Uint64("num_gc", uint64(m.NumGC)),
		)
	}
}
