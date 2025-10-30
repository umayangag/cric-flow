package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/umayangag/cric-app/go-app/internal/features"
)

func main() {
	var season string
	flag.StringVar(&season, "season", "2019", "season name or year (for seasonal form)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// In a full implementation, we might accept which computations to run via flags.
	if err := features.ComputeSeasonalForm(ctx, season); err != nil {
		log.Fatalf("compute seasonal form failed: %v", err)
	}
	if err := features.ComputeVenueEffects(ctx); err != nil {
		log.Fatalf("compute venue effects failed: %v", err)
	}
	if err := features.ComputeOppositionEffects(ctx); err != nil {
		log.Fatalf("compute opposition effects failed: %v", err)
	}
	if err := features.UpdatePlayerConsistency(ctx); err != nil {
		log.Fatalf("update player consistency failed: %v", err)
	}

	log.Println("precompute finished (placeholders)")
}
