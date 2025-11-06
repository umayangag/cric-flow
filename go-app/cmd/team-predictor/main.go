// Command team-predictor builds a team and queries the ML service for win probability.
package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
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

	// Read the pool.csv file
	poolFile, err := os.Open("../ml-service/ml/pool.csv")
	if err != nil {
		log.Fatalf("failed to open pool.csv: %v", err)
	}
	defer func() {
		if err := poolFile.Close(); err != nil {
			log.Printf("close pool.csv: %v", err)
		}
	}()

	reader := csv.NewReader(poolFile)
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("failed to read pool.csv: %v", err)
	}

	// Parse the CSV data into a slice of PlayerPrediction structs
	var players []predictor.PlayerPrediction
	header := records[0]
	for _, record := range records[1:] {
		player := predictor.PlayerPrediction{}
		for i, value := range record {
			floatValue, err := strconv.ParseFloat(value, 64)
			if err != nil {
				// Handle non-numeric values (like player_name)
				if header[i] == "player_name" {
					player.PlayerName = value
				}
				continue
			}
			switch header[i] {
			case "runs_scored":
				player.RunsScored = floatValue
			case "balls_faced":
				player.BallsFaced = floatValue
			case "fours_scored":
				player.FoursScored = floatValue
			case "sixes_scored":
				player.SixesScored = floatValue
			case "batting_position":
				player.BattingPosition = floatValue
			case "strike_rate":
				player.StrikeRate = floatValue
			case "runs_conceded":
				player.RunsConceded = floatValue
			case "deliveries":
				player.Deliveries = floatValue
			case "wickets_taken":
				player.WicketsTaken = floatValue
			case "econ":
				player.Econ = floatValue
			case "winning_probability":
				player.WinningProbability = floatValue
			}
		}
		players = append(players, player)
	}

	// Calculate overall performance (currently not used directly; kept for future metrics)
	_ = predictor.CalculateOverallPerformance(players, matchID)

	// Predict winning probability
	mlClient := mlclient.New()
	predictions, err := mlClient.PredictWin(ctx, players)
	if err != nil {
		log.Fatalf("failed to predict win: %v", err)
	}

	// Sort players by winning probability
	sort.Slice(predictions, func(i, j int) bool {
		return predictions[i].WinningProbability > predictions[j].WinningProbability
	})

	// Select top 11 players
	selectedPlayers := predictions[:cfg.Predictor.TeamSize]

	// Print the selected team
	fmt.Printf("Team for match %d (bat=%d, bowl=%d)\n", matchID, wantBatters, wantBowlers)
	fmt.Println("-------------------------------------")
	for _, p := range selectedPlayers {
		fmt.Printf(" - %s (Winning Probability: %.2f)\n", p.PlayerName, p.WinningProbability)
	}
}
