// Package selection provides an end-to-end team selection pipeline analogous to src/team_selection/select_pool.py
package selection

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/predictor"
)

// SelectionOptions controls constraints for picking the final XI.
type SelectionOptions struct {
	TeamSize      int  // number of players to pick (default 11 if <=0)
	MinBowlers    int  // minimum number of bowlers to include (heuristic: deliveries>0 or econ>0)
	RequireKeeper bool // require at least one wicket-keeper (not enforced in CSV mode; placeholder)
}

// SelectionResult is the outcome from the selection pipeline.
type SelectionResult struct {
	Players            []predictor.PlayerPrediction // selected XI, sorted by winning probability desc
	TeamWinProbability float64                      // mean of per-player winning_probability
}

// SelectTeamFromCSV performs an end-to-end selection by reading a prepared pool CSV (same schema
// used by the prototype), calling the ML service win predictor, applying simple constraints, and
// returning the best XI. This mirrors the orchestration in the Python prototype's __main__ block.
//
// poolCSV schema is expected to include columns like: player_name, runs_scored, balls_faced,
// fours_scored, sixes_scored, batting_position, strike_rate, runs_conceded, deliveries,
// wickets_taken, econ. Extra columns are ignored.
func SelectTeamFromCSV(ctx context.Context, poolCSV string, matchID int64, format, season string, opts SelectionOptions) (SelectionResult, error) {
	if opts.TeamSize <= 0 {
		opts.TeamSize = 11
	}
	if opts.MinBowlers < 0 {
		opts.MinBowlers = 0
	}

	file, err := os.Open(poolCSV)
	if err != nil {
		return SelectionResult{}, fmt.Errorf("open pool csv: %w", err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return SelectionResult{}, fmt.Errorf("read pool csv: %w", err)
	}
	if len(records) == 0 {
		return SelectionResult{}, errors.New("pool csv is empty")
	}
	header := records[0]

	var players []predictor.PlayerPrediction
	for _, rec := range records[1:] {
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
				// ignore extra columns
			}
		}
		players = append(players, p)
	}
	if len(players) == 0 {
		return SelectionResult{}, errors.New("no players parsed from CSV")
	}

	cli := mlclient.New()
	preds, err := cli.PredictWin(ctx, players)
	if err != nil {
		return SelectionResult{}, fmt.Errorf("predict win: %w", err)
	}
	if len(preds) < opts.TeamSize {
		return SelectionResult{}, fmt.Errorf("pool too small: have %d players, need %d", len(preds), opts.TeamSize)
	}

	// Sort by winning probability desc
	sort.Slice(preds, func(i, j int) bool {
		return preds[i].WinningProbability > preds[j].WinningProbability
	})

	// Enforce minimum bowlers by heuristic: deliveries>0 OR econ>0 indicates bowling capability.
	selected := make([]predictor.PlayerPrediction, 0, opts.TeamSize)
	bowlers := 0
	for _, p := range preds {
		if len(selected) >= opts.TeamSize {
			break
		}
		selected = append(selected, p)
		if p.Deliveries > 0 || p.Econ > 0 {
			bowlers++
		}
	}
	// If the selected set does not meet MinBowlers, try to swap in additional bowlers from the remainder.
	if bowlers < opts.MinBowlers {
		remaining := preds[opts.TeamSize:]
		for _, cand := range remaining {
			if bowlers >= opts.MinBowlers {
				break
			}
			if !(cand.Deliveries > 0 || cand.Econ > 0) {
				continue
			}
			// find the lowest-ranked non-bowler in selected to replace
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
	}

	// Compute team average win probability
	var sum float64
	for _, p := range selected {
		sum += p.WinningProbability
	}
	avg := 0.0
	if len(selected) > 0 {
		avg = sum / float64(len(selected))
	}

	// Keep selected sorted by prob desc
	sort.Slice(selected, func(i, j int) bool { return selected[i].WinningProbability > selected[j].WinningProbability })

	return SelectionResult{Players: selected, TeamWinProbability: avg}, nil
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
