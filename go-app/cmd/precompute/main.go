package main

import (
	"context"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/features"
)

func main() {
	var season string
	var format string
	var formats string
	var allFormats bool
	flag.StringVar(&season, "season", "2019", "season name or year (for seasonal form), empty for all")
	flag.StringVar(&format, "format", "", "single format code (TEST, ODI, T20, T20I)")
	flag.StringVar(&formats, "formats", "", "comma-separated list of format codes")
	flag.BoolVar(&allFormats, "all-formats", false, "run for all known formats from config")
	flag.Parse()

	cfg := config.Load()
	timeout := 5 * time.Minute
	if cfg.Features.PrecomputeTimeoutMs > 0 {
		timeout = time.Duration(cfg.Features.PrecomputeTimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Decide which formats to run
	var todo []string
	if allFormats {
		todo = []string{"TEST", "ODI", "T20", "T20I"}
	} else if formats != "" {
		for _, c := range strings.Split(formats, ",") {
			c = strings.TrimSpace(strings.ToUpper(c))
			if c != "" {
				todo = append(todo, c)
			}
		}
	} else if format != "" {
		todo = []string{strings.ToUpper(strings.TrimSpace(format))}
	}

	if len(todo) == 0 {
		// Fallback to all formats
		todo = []string{"TEST", "ODI", "T20", "T20I"}
	}

	for _, f := range todo {
		if err := features.ComputeSeasonalFormFmt(ctx, season, f); err != nil {
			log.Fatalf("compute seasonal form fmt(%s) failed: %v", f, err)
		}
		if err := features.ComputeVenueEffectsFmt(ctx, f); err != nil {
			log.Fatalf("compute venue effects fmt(%s) failed: %v", f, err)
		}
		if err := features.ComputeOppositionEffectsFmt(ctx, f); err != nil {
			log.Fatalf("compute opposition effects fmt(%s) failed: %v", f, err)
		}
	}

	// Global consistency (optionally per-format in future)
	if err := features.UpdatePlayerConsistency(ctx); err != nil {
		log.Fatalf("update player consistency failed: %v", err)
	}

	log.Println("precompute finished (format-aware)")
}
