package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

func main() {
	var matchID int64
	var wantBatters int
	var wantBowlers int
	var formatCode string
	var seasonName string
	flag.Int64Var(&matchID, "match", 0, "match_id to build predictions for")
	// Use 0 defaults to allow config-driven values
	flag.IntVar(&wantBatters, "bat", 0, "number of batters to pick (defaults from config.team.default_batters)")
	flag.IntVar(&wantBowlers, "bowl", 0, "number of bowlers to pick (defaults from config.team.default_bowlers)")
	flag.StringVar(&formatCode, "format", "", "match format code (TEST, ODI, T20, T20I)")
	flag.StringVar(&seasonName, "season", "", "season name (e.g. 2019)")
	flag.Parse()

	cfg := config.Load()
	if matchID == 0 || strings.TrimSpace(formatCode) == "" || strings.TrimSpace(seasonName) == "" {
		fmt.Fprintln(
			os.Stderr,
			"usage: team-predictor -match=<match_id> -format=<CODE> -season=<season> [-bat=N] [-bowl=N]",
		)
		os.Exit(2)
	}
	// Apply config defaults when flags are not provided (0)
	if wantBatters <= 0 {
		if cfg.Team.DefaultBatters > 0 {
			wantBatters = cfg.Team.DefaultBatters
		} else {
			wantBatters = 6
		}
	}
	minB := 5
	if cfg.Team.MinBowlers > 0 {
		minB = cfg.Team.MinBowlers
	}
	if wantBowlers <= 0 {
		if cfg.Team.DefaultBowlers > 0 {
			wantBowlers = cfg.Team.DefaultBowlers
		} else {
			wantBowlers = minB
		}
	}
	if wantBowlers < minB {
		log.Printf("requested bowlers=%d < %d; adjusting to satisfy minimum", wantBowlers, minB)
		wantBowlers = minB
	}

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
	defer poolFile.Close()

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

	// Calculate overall performance
	team := predictor.CalculateOverallPerformance(players, matchID)

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
