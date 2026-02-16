// Package predictteam implements future-match team selection: pick best 11 for each team
// using ML predictions (batting, bowling, fielding) with venue, opposition, and weather context.
package predictteam

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/teamselect"
)

// Input defines the request for future-match team selection.
type Input struct {
	Format        string    `json:"format"`
	Team1         string    `json:"team1"`
	Team2         string    `json:"team2"`
	Venue         string    `json:"venue,omitempty"` // venue name; empty = unknown venue
	MatchDate     time.Time `json:"match_date"`
	SeasonID      *int64    `json:"season_id,omitempty"`
	ExtraTeam1    []int64   `json:"extra_team1,omitempty"`    // extra player IDs for team1 (e.g. IPL auction)
	ExtraTeam2    []int64   `json:"extra_team2,omitempty"`    // extra player IDs for team2
	MinBowlers    int       `json:"min_bowlers,omitempty"`    // default 5
	RequireKeeper bool      `json:"require_keeper,omitempty"` // default true
}

// SelectedPlayer is one player in the selected XI with predictions.
type SelectedPlayer struct {
	PlayerID   int64   `json:"player_id"`
	PlayerName string  `json:"player_name"`
	Runs       float64 `json:"runs"`
	Wickets    float64 `json:"wickets"`
	Economy    float64 `json:"economy"`
	Catches    float64 `json:"catches"`
	RunOuts    float64 `json:"run_outs"`
}

// Result holds the best 11 for each team.
type Result struct {
	Team1 []SelectedPlayer `json:"team1"`
	Team2 []SelectedPlayer `json:"team2"`
}

// MLPredictor provides player predictions from features (e.g. via ML backtest endpoint).
type MLPredictor interface {
	PredictPlayers(
		ctx context.Context,
		cutoff time.Time,
		format string,
		playerIDs []int64,
		features map[int64]map[string]float64,
	) (map[int64]PlayerPred, error)
}

// PlayerPred holds ML prediction output.
type PlayerPred struct {
	Runs    float64
	Wickets float64
	Economy float64
	Catches float64
	RunOuts float64
}

// PredictTeams runs the full pipeline: pool, features, ML predict, team select.
func PredictTeams(ctx context.Context, input Input, predictor MLPredictor) (*Result, error) {
	if input.MinBowlers <= 0 {
		input.MinBowlers = 5
	}
	format := strings.ToUpper(strings.TrimSpace(input.Format))
	team1 := strings.TrimSpace(input.Team1)
	team2 := strings.TrimSpace(input.Team2)
	if format == "" || team1 == "" || team2 == "" {
		return nil, fmt.Errorf("format, team1, team2 are required")
	}
	cutoff := input.MatchDate.Truncate(24 * time.Hour)

	// Resolve venue ID
	var venueID *int64
	if input.Venue != "" {
		id, err := db.GetGlobalCache().GetVenueID(ctx, input.Venue)
		if err == nil && id != 0 {
			venueID = &id
		}
	}

	// Opposition IDs for feature context (when team1 bats, they face team2)
	opp1ID, _ := db.GetGlobalCache().GetOppositionID(ctx, team1)
	opp2ID, _ := db.GetGlobalCache().GetOppositionID(ctx, team2)

	// Player pools
	pool1, err := db.ListPlayerPoolByTeam(ctx, format, team1, cutoff, input.ExtraTeam1)
	if err != nil {
		return nil, fmt.Errorf("team1 pool: %w", err)
	}
	pool2, err := db.ListPlayerPoolByTeam(ctx, format, team2, cutoff, input.ExtraTeam2)
	if err != nil {
		return nil, fmt.Errorf("team2 pool: %w", err)
	}
	if len(pool1) < 11 {
		return nil, fmt.Errorf("team1 has only %d players, need at least 11", len(pool1))
	}
	if len(pool2) < 11 {
		return nil, fmt.Errorf("team2 has only %d players, need at least 11", len(pool2))
	}

	// Features and predictions for team1 (opposition = team2)
	ids1 := make([]int64, 0, len(pool1))
	for _, p := range pool1 {
		ids1 = append(ids1, p.PlayerID)
	}
	feats1, err := exportqueries.ComputeFeaturesAtCutoffForFutureMatch(
		ctx,
		cutoff,
		format,
		venueID,
		opp2ID,
		input.SeasonID,
		ids1,
	)
	if err != nil {
		return nil, fmt.Errorf("team1 features: %w", err)
	}
	preds1, err := predictor.PredictPlayers(ctx, cutoff, format, ids1, feats1)
	if err != nil {
		return nil, fmt.Errorf("team1 predict: %w", err)
	}

	// Features and predictions for team2 (opposition = team1)
	ids2 := make([]int64, 0, len(pool2))
	for _, p := range pool2 {
		ids2 = append(ids2, p.PlayerID)
	}
	feats2, err := exportqueries.ComputeFeaturesAtCutoffForFutureMatch(
		ctx,
		cutoff,
		format,
		venueID,
		opp1ID,
		input.SeasonID,
		ids2,
	)
	if err != nil {
		return nil, fmt.Errorf("team2 features: %w", err)
	}
	preds2, err := predictor.PredictPlayers(ctx, cutoff, format, ids2, feats2)
	if err != nil {
		return nil, fmt.Errorf("team2 predict: %w", err)
	}

	// Build teamselect pool and select
	tsPool1 := buildTeamSelectPool(pool1, preds1)
	tsPool2 := buildTeamSelectPool(pool2, preds2)

	sel1, err := teamselect.Select(tsPool1, teamselect.DefaultWeights(), teamselect.Constraints{
		Size:          11,
		MinBowlers:    input.MinBowlers,
		RequireKeeper: input.RequireKeeper,
	})
	if err != nil {
		return nil, fmt.Errorf("team1 select: %w", err)
	}
	sel2, err := teamselect.Select(tsPool2, teamselect.DefaultWeights(), teamselect.Constraints{
		Size:          11,
		MinBowlers:    input.MinBowlers,
		RequireKeeper: input.RequireKeeper,
	})
	if err != nil {
		return nil, fmt.Errorf("team2 select: %w", err)
	}

	// Map selected names back to player IDs and predictions
	nameToPred1 := make(map[string]PlayerPred)
	for _, p := range pool1 {
		pr := preds1[p.PlayerID]
		nameToPred1[p.PlayerName] = pr
	}
	nameToPred2 := make(map[string]PlayerPred)
	for _, p := range pool2 {
		pr := preds2[p.PlayerID]
		nameToPred2[p.PlayerName] = pr
	}
	nameToID1 := make(map[string]int64)
	for _, p := range pool1 {
		nameToID1[p.PlayerName] = p.PlayerID
	}
	nameToID2 := make(map[string]int64)
	for _, p := range pool2 {
		nameToID2[p.PlayerName] = p.PlayerID
	}

	result := &Result{
		Team1: make([]SelectedPlayer, 0, len(sel1)),
		Team2: make([]SelectedPlayer, 0, len(sel2)),
	}
	for _, p := range sel1 {
		pr := nameToPred1[p.Name]
		result.Team1 = append(result.Team1, SelectedPlayer{
			PlayerID:   nameToID1[p.Name],
			PlayerName: p.Name,
			Runs:       pr.Runs,
			Wickets:    pr.Wickets,
			Economy:    pr.Economy,
			Catches:    pr.Catches,
			RunOuts:    pr.RunOuts,
		})
	}
	for _, p := range sel2 {
		pr := nameToPred2[p.Name]
		result.Team2 = append(result.Team2, SelectedPlayer{
			PlayerID:   nameToID2[p.Name],
			PlayerName: p.Name,
			Runs:       pr.Runs,
			Wickets:    pr.Wickets,
			Economy:    pr.Economy,
			Catches:    pr.Catches,
			RunOuts:    pr.RunOuts,
		})
	}
	return result, nil
}

func buildTeamSelectPool(pool []db.PlayerPoolRow, preds map[int64]PlayerPred) []teamselect.Player {
	out := make([]teamselect.Player, 0, len(pool))
	for _, p := range pool {
		pr := preds[p.PlayerID]
		batScore := normalizeBatScore(pr.Runs)
		bowlScore := normalizeBowlScore(pr.Wickets, pr.Economy)
		fieldScore := normalizeFieldScore(pr.Catches, pr.RunOuts)
		isBowler := p.BowlingConsistency.Valid && p.BowlingConsistency.Float64 > 0
		out = append(out, teamselect.Player{
			Name:       p.PlayerName,
			IsBowler:   isBowler,
			IsKeeper:   p.IsWicketKeeper == 1,
			BatScore:   batScore,
			BowlScore:  bowlScore,
			FieldScore: fieldScore,
		})
	}
	return out
}

func normalizeBatScore(runs float64) float64 {
	// T20/ODI: typical max ~80-100 per player
	return math.Min(1, runs/80)
}

func normalizeBowlScore(wickets, economy float64) float64 {
	// Wickets good, economy: lower is better (12 = poor, 6 = excellent)
	wickPart := math.Min(1, wickets/5)
	econPart := math.Max(0, 1-(economy/12))
	return (wickPart + econPart) / 2
}

func normalizeFieldScore(catches, runOuts float64) float64 {
	// Catches + run_outs, typical max ~3-4 per match
	return math.Min(1, (catches+runOuts*1.5)/5)
}
