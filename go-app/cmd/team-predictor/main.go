// Command team-predictor builds a team and queries the ML service for win probability.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

func main() {
	cfg := config.Load()

	opts, err := parseFlags(os.Args[1:], cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	matchID := opts.matchID
	wantBatters := opts.batters
	wantBowlers := opts.bowlers

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := db.Connect(ctx); err != nil {
		log.Fatalf("db connect failed: %v", err)
	}

	// Read the pool.csv file and parse players via pure helper
	poolFile, err := os.Open("../ml-service/ml/pool.csv")
	if err != nil {
		log.Fatalf("failed to open pool.csv: %v", err)
	}
	defer func() {
		if err := poolFile.Close(); err != nil {
			log.Printf("close pool.csv: %v", err)
		}
	}()

	players, err := parsePlayersCSV(poolFile)
	if err != nil {
		log.Fatalf("failed to parse pool.csv: %v", err)
	}

	// Calculate overall performance (currently not used directly; kept for future metrics)
	_ = predictor.CalculateOverallPerformance(players, matchID)

	// Predict winning probability and build team using pure helper
	mlClient := mlclient.New()
	selectedPlayers, err := buildTeam(ctx, mlClient, players, cfg.Predictor.TeamSize)
	if err != nil {
		log.Fatalf("failed to predict win: %v", err)
	}

	// Print the selected team
	fmt.Printf("Team for match %d (bat=%d, bowl=%d)\n", matchID, wantBatters, wantBowlers)
	fmt.Println("-------------------------------------")
	for _, p := range selectedPlayers {
		fmt.Printf(" - %s (Winning Probability: %.2f)\n", p.PlayerName, p.WinningProbability)
	}
}
