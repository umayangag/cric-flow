package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

// parsePlayersCSV reads CSV data from r and returns player predictions.
// It expects a header row whose columns include at least:
//   - player_name (string)
//   - runs_scored, balls_faced, fours_scored, sixes_scored, batting_position, strike_rate
//   - runs_conceded, deliveries, wickets_taken, econ, winning_probability
//
// Unknown columns are ignored. Non-numeric values in numeric columns are skipped.
func parsePlayersCSV(r io.Reader) ([]predictor.PlayerPrediction, error) {
	cr := csv.NewReader(r)
	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("empty csv")
	}
	header := records[0]
	players := make([]predictor.PlayerPrediction, 0, len(records)-1)
	for _, rec := range records[1:] {
		var p predictor.PlayerPrediction
		for i, val := range rec {
			if i >= len(header) {
				continue
			}
			name := header[i]
			if name == "player_name" {
				p.PlayerName = val
				continue
			}
			f, err := strconv.ParseFloat(val, 64)
			if err != nil {
				// ignore non-numeric for numeric fields
				continue
			}
			switch name {
			case "runs_scored":
				p.RunsScored = f
			case "balls_faced":
				p.BallsFaced = f
			case "fours_scored":
				p.FoursScored = f
			case "sixes_scored":
				p.SixesScored = f
			case "batting_position":
				p.BattingPosition = f
			case "strike_rate":
				p.StrikeRate = f
			case "runs_conceded":
				p.RunsConceded = f
			case "deliveries":
				p.Deliveries = f
			case "wickets_taken":
				p.WicketsTaken = f
			case "econ":
				p.Econ = f
			case "winning_probability":
				p.WinningProbability = f
			}
		}
		players = append(players, p)
	}
	return players, nil
}
