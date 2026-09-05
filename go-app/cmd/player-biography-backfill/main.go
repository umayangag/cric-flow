// Command player-biography-backfill acquires player biographies from Wikidata and
// measures how much of the archive they cover (X-1a in docs/EXTERNAL_DATA_PLAN.md).
//
// It is a step beside the cadence, not in it. Biographies change on the scale of a career
// — a date of birth never, a bowling style rarely — so re-acquiring them on every data
// refresh would be thirty requests a week to answer a question whose answer did not
// change. Run it when the player registry grows enough to matter, or weekly beside the
// cadence if that is simpler to schedule.
//
// The run is resumable: every batch's answers, including its misses, are appended to a
// local cache before the next batch starts, so an interrupted run is resumed by running
// the same command again.
//
// With -offline it rebuilds the table from the snapshots committed under reference-data/
// and makes no network call at all. That is the recovery path after a purge, and it is
// the reason those snapshots are tracked: re-acquiring them is a rate-limited pass over
// every player in the archive.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/logger"
	"github.com/umayangag/cric-flow/go-app/internal/services/biographybackfill"
)

func main() { os.Exit(run()) }

func run() int {
	fs := flag.NewFlagSet("player-biography-backfill", flag.ContinueOnError)
	options, err := biographybackfill.ParseArgs(fs, os.Args[1:])
	if err != nil {
		slog.Error("flag parsing failed", slog.Any("err", err))
		return 2
	}

	logger.SetupFromEnv()
	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(baseCtx, options.Timeout)
	defer cancel()

	if _, err := db.Connect(ctx); err != nil {
		slog.Error("db connect failed", slog.Any("err", err))
		return 1
	}
	if err := db.RunMigrations(ctx, "./migrations"); err != nil {
		slog.Error("migrations failed", slog.Any("err", err))
		return 1
	}

	store := db.NewPlayerBiographyStore()
	if !options.ReportOnly {
		if err := acquire(ctx, store, options); err != nil {
			slog.Error("player biography backfill failed", slog.Any("err", err))
			return 1
		}
	}
	if err := report(ctx, store, options); err != nil {
		slog.Error("writing the coverage report failed", slog.Any("err", err))
		return 1
	}
	return 0
}

// acquire runs the Wikidata pass.
func acquire(
	ctx context.Context,
	store biography.Store,
	options biographybackfill.Options,
) error {
	client := &http.Client{Timeout: options.Timeout}
	register, err := biographybackfill.LoadRegister(client, options.UserAgent, options.Register)
	if err != nil {
		return err
	}
	overrides, err := biographybackfill.LoadOverrides(options.OverridesFile)
	if err != nil {
		return err
	}
	cache, err := biography.OpenCache(options.CacheFile)
	if err != nil {
		return err
	}
	slog.Info("player biographies: sources loaded",
		slog.Int("register_entries", len(register)),
		slog.Int("overrides", len(overrides)),
		slog.Int("cached_lookups", cache.Len()),
		slog.String("license", biography.SourceLicense))

	result, err := biography.Run(ctx, store, lookuperFor(options, client), biography.Options{
		Register:  register,
		Overrides: overrides,
		Cache:     cache,
		BatchSize: options.BatchSize,
		Offline:   options.Offline,
	})
	if err != nil {
		return err
	}
	slog.Info("player biographies: acquisition finished",
		slog.Bool("offline", options.Offline),
		slog.Int("players", result.Players),
		slog.Int("with_cricinfo_id", result.WithCricinfoID),
		slog.Int("asked_now", result.AskedNow),
		slog.Int("from_cache", result.FromCache),
		slog.Int("unanswered", result.Unanswered),
		slog.Int("matched", result.Matched),
		slog.Int("overridden", result.Overridden))
	return nil
}

// lookuperFor returns the Wikidata client, or nothing at all for an offline restore.
//
// An offline run gets no client rather than a client it promises not to use: the promise
// that the restore cannot reach Wikidata is then structural, not a comment.
func lookuperFor(options biographybackfill.Options, client *http.Client) biography.Lookuper {
	if options.Offline {
		return nil
	}
	return &biography.SPARQLClient{
		Endpoint:  options.Endpoint,
		UserAgent: options.UserAgent,
		HTTP:      client,
		Pause:     options.Pause,
	}
}

// report measures what is stored and writes it out.
func report(
	ctx context.Context,
	store biography.Store,
	options biographybackfill.Options,
) error {
	coverage, err := store.Coverage(ctx)
	if err != nil {
		return err
	}
	if options.ReportFile != "" {
		if err := biographybackfill.WriteFile(
			options.ReportFile, []byte(biography.RenderMarkdown(coverage))); err != nil {
			return err
		}
		slog.Info("player biographies: coverage report written",
			slog.String("path", options.ReportFile))
	}
	if options.JSONReportFile != "" {
		encoded, err := json.MarshalIndent(coverage, "", "  ")
		if err != nil {
			return err
		}
		if err := biographybackfill.WriteFile(options.JSONReportFile, append(encoded, '\n')); err != nil {
			return err
		}
		slog.Info("player biographies: coverage JSON written",
			slog.String("path", options.JSONReportFile))
	}
	slog.Info("player biographies: coverage",
		slog.Int64("appearances", coverage.Total.Appearances),
		slog.Float64("matched_pct", biography.Share(coverage.Total.Matched, coverage.Total.Appearances)),
		slog.Float64("dob_pct", biography.Share(coverage.Total.BirthDate, coverage.Total.Appearances)),
		slog.Float64("style_pct", biography.Share(coverage.Total.BowlingStyle, coverage.Total.Appearances)))
	return nil
}
