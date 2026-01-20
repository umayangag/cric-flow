// Package selection provides an end-to-end team selection pipeline analogous to src/team_selection/select_pool.py
package selection

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

// Options controls constraints for picking the final XI.
type Options struct {
	TeamSize      int  // number of players to pick (default 11 if <=0)
	MinBowlers    int  // minimum number of bowlers to include (heuristic: deliveries>0 or econ>0)
	RequireKeeper bool // require at least one wicket-keeper (not enforced in CSV mode; placeholder)
}

// Result is the outcome from the selection pipeline.
type Result struct {
	Players            []predictor.PlayerPrediction // selected XI, sorted by winning probability desc
	TeamWinProbability float64                      // mean of per-player winning_probability
}

// SelectTeamFromCSV performs an end-to-end selection by reading a prepared pool CSV (same schema
// used by the prototype), calling the mlCleint service win predictor, applying simple constraints, and
// returning the best XI. This mirrors the orchestration in the Python prototype's __main__ block.
//
// poolCSV schema is expected to include columns like: player_name, runs_scored, balls_faced,
// fours_scored, sixes_scored, batting_position, strike_rate, runs_conceded, deliveries,
// wickets_taken, econ. Extra columns are ignored.
func SelectTeamFromCSV(
	ctx context.Context,
	poolCSV string,
	_ int64,
	_, _ string,
	opts Options,
) (Result, error) {
	// Normalize options
	if opts.TeamSize <= 0 {
		opts.TeamSize = 11
	}
	if opts.MinBowlers < 0 {
		opts.MinBowlers = 0
	}

	// Read and parse CSV into player feature rows
	records, err := readAllCSV(poolCSV)
	if err != nil {
		return Result{}, err
	}
	if len(records) == 0 {
		return Result{}, errors.New("pool csv is empty")
	}
	header := records[0]
	players := parsePlayersFromCSV(header, records[1:])
	if len(players) == 0 {
		return Result{}, errors.New("no players parsed from CSV")
	}

	cli := mlclient.New()
	preds, err := cli.PredictWin(ctx, players)
	if err != nil {
		return Result{}, fmt.Errorf("predict win: %w", err)
	}
	// Select top team respecting minimum bowlers
	selected, err := selectTopWithMinBowlers(preds, opts.TeamSize, opts.MinBowlers)
	if err != nil {
		return Result{}, err
	}
	avg := computeAverageWinProbability(selected)
	// Ensure selected is sorted by probability desc
	sort.Slice(selected, func(i, j int) bool { return selected[i].WinningProbability > selected[j].WinningProbability })
	return Result{Players: selected, TeamWinProbability: avg}, nil
}

func parseF64(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// readAllCSV opens a CSV file path and returns all records.
func readAllCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open pool csv: %w", err)
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read pool csv: %w", err)
	}
	return recs, nil
}

// parsePlayersFromCSV maps CSV header+rows into predictor.PlayerPrediction rows.
func parsePlayersFromCSV(header []string, rows [][]string) []predictor.PlayerPrediction {
	players := make([]predictor.PlayerPrediction, 0, len(rows))
	for _, rec := range rows {
		p := predictor.PlayerPrediction{}
		for i := range header {
			if i >= len(rec) {
				continue
			}
			col := header[i]
			val := rec[i]
			switch col {
			case "player_name":
				p.PlayerName = val
			case "runs_scored":
				p.RunsScored = parseF64(val)
			case "balls_faced":
				p.BallsFaced = parseF64(val)
			case "fours_scored":
				p.FoursScored = parseF64(val)
			case "sixes_scored":
				p.SixesScored = parseF64(val)
			case "batting_position":
				p.BattingPosition = parseF64(val)
			case "strike_rate":
				p.StrikeRate = parseF64(val)
			case "runs_conceded":
				p.RunsConceded = parseF64(val)
			case "deliveries":
				p.Deliveries = parseF64(val)
			case "wickets_taken":
				p.WicketsTaken = parseF64(val)
			case "econ":
				p.Econ = parseF64(val)
			case "winning_probability":
				p.WinningProbability = parseF64(val)
			default:
				// ignore unknown cols
			}
		}
		players = append(players, p)
	}
	return players
}

// selectTopWithMinBowlers selects the top teamSize by probability while ensuring at least minBowlers
// using the heuristic: bowler if deliveries>0 OR econ>0.
func selectTopWithMinBowlers(preds []predictor.PlayerPrediction, teamSize int, minBowlers int) ([]predictor.PlayerPrediction, error) {
	if len(preds) < teamSize {
		return nil, fmt.Errorf("pool too small: have %d players, need %d", len(preds), teamSize)
	}
	// Sort by prob desc
	sort.Slice(preds, func(i, j int) bool { return preds[i].WinningProbability > preds[j].WinningProbability })
	selected := make([]predictor.PlayerPrediction, 0, teamSize)
	bowlers := 0
	for _, p := range preds {
		if len(selected) >= teamSize {
			break
		}
		selected = append(selected, p)
		if p.Deliveries > 0 || p.Econ > 0 {
			bowlers++
		}
	}
	if bowlers >= minBowlers {
		return selected, nil
	}
	// Try swaps from the remainder in descending order
	remaining := preds[teamSize:]
	for _, cand := range remaining {
		if bowlers >= minBowlers {
			break
		}
		if !(cand.Deliveries > 0 || cand.Econ > 0) {
			continue
		}
		// replace the lowest-ranked non-bowler
		idx := -1
		for i := len(selected) - 1; i >= 0; i-- {
			if !(selected[i].Deliveries > 0 || selected[i].Econ > 0) {
				idx = i
				break
			}
		}
		if idx >= 0 {
			selected[idx] = cand
			bowlers++
		}
	}
	return selected, nil
}

func computeAverageWinProbability(players []predictor.PlayerPrediction) float64 {
	if len(players) == 0 {
		return 0
	}
	var sum float64
	for _, p := range players {
		sum += p.WinningProbability
	}
	return sum / float64(len(players))
}
