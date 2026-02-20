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

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/logger"
	"github.com/umayangag/cric-flow/go-app/internal/mlclient"
	apipkg "github.com/umayangag/cric-flow/go-app/internal/server"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
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

	// Cancel only stale IN_PROGRESS runs (started longer ago than threshold), so we don't
	// cancel a pipeline that another instance (B) is currently running when this instance (A) restarts.
	staleCancelAge := trackingStaleCancelAge()
	if n, err := tracking.CancelStaleInProgressMigrations(ctx, "interrupted (server restart or crash)", staleCancelAge); err != nil {
		slog.Warn("failed to cancel stale in-progress migrations", slog.Any("err", err))
	} else if n > 0 {
		slog.Info("cancelled stale in-progress pipeline runs", slog.Int("count", n), slog.Duration("stale_older_than", staleCancelAge))
	}

	// Context cancelled on SIGTERM/SIGINT so in-flight pipeline jobs exit gracefully
	jobCtx, cancelJob := context.WithCancel(context.Background())
	defer cancelJob()

	// Initialize long-lived dependencies
	client := mlclient.New()
	server := apipkg.NewApp(jobCtx, client)

	// Build router with dependencies
	r := apipkg.NewRouter(server)

	cfg := config.Load()
	addr := config.ServerListenAddress(cfg)
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}
	slog.Info("API listening", slog.String("address", addr))

	readSec := config.ServerHTTPReadTimeoutSec(cfg)
	writeSec := config.ServerHTTPWriteTimeoutSec(cfg)
	idleSec := config.ServerHTTPIdleTimeoutSec(cfg)
	// Use http.Server to set timeouts (gosec G114)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadTimeout:       time.Duration(readSec) * time.Second,
		ReadHeaderTimeout: time.Duration(readSec) * time.Second,
		WriteTimeout:      time.Duration(writeSec) * time.Second,
		IdleTimeout:       time.Duration(idleSec) * time.Second,
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
		cancelJob() // cancel pipeline job context so in-flight jobs see ctx.Done() and exit
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()
		shutdownErr := srv.Shutdown(shutdownCtx)
		db.Close() // Ensure DB connection is closed after shutdown attempt
		if shutdownErr != nil {
			slog.Error("server shutdown failed (timeout or error)", slog.Any("err", shutdownErr))
			return 1
		}
		slog.Info("server shutdown complete")
		return 0
	case err := <-serverErr:
		if err != nil {
			slog.Error("server exited with error", slog.Any("err", err))
			db.Close()
			return 1
		}
		db.Close() // Drain pool and flush logs on normal server exit
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

// trackingStaleCancelAge returns how old an IN_PROGRESS run must be to be cancelled on startup
// (so we don't cancel another instance's active run). TRACKING_STALE_CANCEL_AGE (e.g. 24h, 30m); default 24h.
func trackingStaleCancelAge() time.Duration {
	const defaultAge = 24 * time.Hour
	s := os.Getenv("TRACKING_STALE_CANCEL_AGE")
	if s == "" {
		return defaultAge
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		slog.Warn(
			"invalid or non-positive value for TRACKING_STALE_CANCEL_AGE, using default",
			"value",
			s,
			"err",
			err,
			"default",
			defaultAge,
		)
		return defaultAge
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
