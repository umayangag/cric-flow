// Package predictteam implements future-match team selection: pick best 11 for each team
// using ML predictions (batting, bowling, fielding) with venue, opposition, and weather context.
package predictteam

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-flow/go-app/internal/features"
	"github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

// WeatherInput holds optional weather conditions for the match (forecast or historical average).
// When provided, these override the default 0 values in feature computation.
type WeatherInput struct {
	Temp     float64 `json:"temp,omitempty"`     // Celsius
	Humidity float64 `json:"humidity,omitempty"` // 0-100
	Wind     float64 `json:"wind,omitempty"`
	Rain     float64 `json:"rain,omitempty"`
	Cloud    float64 `json:"cloud,omitempty"`
	Pressure float64 `json:"pressure,omitempty"`
}

// Input defines the request for future-match team selection.
type Input struct {
	Format              string        `json:"format"`
	Team1               string        `json:"team1"`
	Team2               string        `json:"team2"`
	Venue               string        `json:"venue,omitempty"` // venue name; empty = unknown venue
	MatchDate           time.Time     `json:"match_date"`
	SeasonID            *int64        `json:"season_id,omitempty"`
	Weather             *WeatherInput `json:"weather,omitempty"`               // optional; when set, used in features
	ExtraTeam1          []int64       `json:"extra_team1,omitempty"`           // extra player IDs for team1 (e.g. IPL auction)
	ExtraTeam2          []int64       `json:"extra_team2,omitempty"`           // extra player IDs for team2
	OppositionPlayerIDs []int64       `json:"opposition_player_ids,omitempty"` // optional; for future batter-bowler matchup features
	MinBowlers               int  `json:"min_bowlers,omitempty"`               // default 5
	RequireKeeper            bool `json:"require_keeper,omitempty"`            // default true
	UseUnifiedModel          bool `json:"use_unified_model,omitempty"`         // when true, use legacy unified model instead of format-specific
	UseReconciledScorecard   bool `json:"use_reconciled_scorecard,omitempty"` // when true, primary scorecard is from generate-match (reconciled)
	IncludeBothScorecards    bool `json:"include_both_scorecards,omitempty"`   // when true, return both standard and reconciled scorecards for comparison
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

// ScorecardSummary holds predicted innings totals and winner for an upcoming match.
// When the win model is used, PredictedWinner and optionally Innings1Total/Innings2Total are consistent with Team1WinProbability (feedback).
type ScorecardSummary struct {
	Innings1Total       float64 `json:"innings1_total"`
	Innings2Total       float64 `json:"innings2_total"`
	PredictedWinner     string  `json:"predicted_winner"`
	Team1WinProbability float64 `json:"team1_win_probability,omitempty"` // from win model when available
	ExtrasInnings1      float64 `json:"extras_innings1,omitempty"`
	ExtrasInnings2      float64 `json:"extras_innings2,omitempty"`
}

// Result holds the best 11 for each team and optional scorecard summary.
type Result struct {
	Team1                     []SelectedPlayer  `json:"team1"`
	Team2                     []SelectedPlayer  `json:"team2"`
	ScorecardSummary          *ScorecardSummary `json:"scorecard_summary,omitempty"`
	ScorecardSummaryReconciled *ScorecardSummary `json:"scorecard_summary_reconciled,omitempty"` // from generate-match when requested
}

// MatchContext is optional context for hybrid reconciliation (innings model rescaling).
// When provided with both teams, ML service rescales predictions so runs and wickets tally.
type MatchContext struct {
	Team1PlayerIDs                                         []int64
	Team2PlayerIDs                                         []int64
	VenueID                                                int64
	SeasonID                                               int64
	FormatID                                               int64
	Team1OppositionID                                      int64 // team2's ID when team1 bats (innings 1)
	Team2OppositionID                                      int64 // team1's ID when team2 bats (innings 2)
	Temp, Wind, Rain, Humidity, Cloud, Pressure, Viscosity int
}

// GenerateMatchFunc is an optional callback to fetch a reconciled match projection (per-player stats,
// innings totals, win probability) from the ML generate-match API. Used when UseReconciledScorecard
// or IncludeBothScorecards is set so the client can compare or switch between standard and reconciled outputs.
type GenerateMatchFunc func(
	ctx context.Context,
	cutoff time.Time,
	format string,
	playerIDs []int64,
	features map[int64]map[string]float64,
	useLatest bool,
	matchCtx *MatchContext,
) (players map[int64]PlayerPred, innings1Runs, innings2Runs float64, winProbTeam1 float64, modelVersion string, err error)

// MLPredictor provides player predictions and optional match-level win probability.
// When matchCtx is non-nil, ML may rescale predictions for consistency (requires innings model).
type MLPredictor interface {
	PredictPlayers(
		ctx context.Context,
		cutoff time.Time,
		format string,
		playerIDs []int64,
		features map[int64]map[string]float64,
		matchCtx *MatchContext,
	) (map[int64]PlayerPred, error)
	// PredictMatchWin returns team1 (batting first) win probability in [0,1].
	// When the win model is not loaded or request fails, returns an error and the caller should use winner-from-totals.
	PredictMatchWin(ctx context.Context, w WinFeatures) (team1WinProbability float64, err error)
}

// WinFeatures holds match-level inputs for the win model (same families as training: format, venue, teams, toss, weather, team consistency/form sums).
type WinFeatures struct {
	FormatID                int
	VenueID                 int
	Team1OppositionID       int
	Team2OppositionID       int
	TossWinnerOppositionID  int
	Temp                    int
	Wind                    int
	Rain                    int
	Humidity                int
	Cloud                   int
	Pressure                int
	Viscosity               int
	Team1BatConsistencySum  float64
	Team1BowlConsistencySum float64
	Team2BatConsistencySum  float64
	Team2BowlConsistencySum float64
	Team1BatFormSum         float64
	Team1BowlFormSum        float64
	Team2BatFormSum         float64
	Team2BowlFormSum        float64
	Format                  string
}

// PlayerPred holds ML prediction output.
type PlayerPred struct {
	Runs    float64
	Wickets float64
	Economy float64
	Catches float64
	RunOuts float64
}

// predictIntermediates holds pool, predictions, and context from the PredictTeams pipeline
// so callers (e.g. PredictTeamsWithSimulation) can reuse them without re-querying DB or ML.
type predictIntermediates struct {
	Pool1, Pool2     []db.PlayerPoolRow
	Preds1, Preds2   map[int64]PlayerPred
	FormatID         int64
	VenueID          *int64
	Extras1, Extras2 float64
	Team1, Team2     string
}

// predictTeamsWithIntermediates runs the full pipeline and returns the result plus
// intermediates (pools, preds, formatID, venueID, extras, team names) for reuse.
// When reconciledGen is non-nil and input requests reconciled/compare scorecards, it is called
// with the selected XI and used to populate ScorecardSummaryReconciled and optionally override the primary summary.
func predictTeamsWithIntermediates(
	ctx context.Context,
	input Input,
	predictor MLPredictor,
	reconciledGen GenerateMatchFunc,
) (*Result, *predictIntermediates, error) {
	if input.MinBowlers <= 0 {
		if cfg := config.Load(); cfg != nil && cfg.Team.MinBowlers > 0 {
			input.MinBowlers = cfg.Team.MinBowlers
		} else {
			input.MinBowlers = config.DefaultMinBowlers
		}
	}
	format := strings.ToUpper(strings.TrimSpace(input.Format))
	team1 := strings.TrimSpace(input.Team1)
	team2 := strings.TrimSpace(input.Team2)
	if format == "" || team1 == "" || team2 == "" {
		err := fmt.Errorf("format, team1, team2 are required")
		slog.Error("predictteam.PredictTeams validation failed", slog.Any("err", err))
		return nil, nil, err
	}
	cutoff := input.MatchDate.Truncate(24 * time.Hour)

	// When UseUnifiedModel is true, pass empty format to ML so it uses the legacy unified model.
	formatForPrediction := format
	if input.UseUnifiedModel {
		formatForPrediction = ""
	}

	formatID, fmtErr := db.GetGlobalCache().GetFormatID(ctx, format)
	if fmtErr != nil {
		slog.Error(
			"predictteam.PredictTeams resolve format failed",
			slog.String("format", format),
			slog.Any("err", fmtErr),
		)
		return nil, nil, fmt.Errorf("resolve format: %w", fmtErr)
	}

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
	opp1IDVal := opp1ID
	opp2IDVal := opp2ID

	// Player pools
	pool1, err := db.ListPlayerPoolByTeam(ctx, format, team1, cutoff, input.ExtraTeam1)
	if err != nil {
		slog.Error("predictteam.PredictTeams team1 pool failed", slog.String("team1", team1), slog.Any("err", err))
		return nil, nil, fmt.Errorf("team1 pool: %w", err)
	}
	pool2, err := db.ListPlayerPoolByTeam(ctx, format, team2, cutoff, input.ExtraTeam2)
	if err != nil {
		slog.Error("predictteam.PredictTeams team2 pool failed", slog.String("team2", team2), slog.Any("err", err))
		return nil, nil, fmt.Errorf("team2 pool: %w", err)
	}
	if len(pool1) < 11 {
		err := fmt.Errorf("team1 has only %d players, need at least 11", len(pool1))
		slog.Error(
			"predictteam.PredictTeams pool size",
			slog.String("team1", team1),
			slog.Int("len", len(pool1)),
			slog.Any("err", err),
		)
		return nil, nil, err
	}
	if len(pool2) < 11 {
		err := fmt.Errorf("team2 has only %d players, need at least 11", len(pool2))
		slog.Error(
			"predictteam.PredictTeams pool size",
			slog.String("team2", team2),
			slog.Int("len", len(pool2)),
			slog.Any("err", err),
		)
		return nil, nil, err
	}

	// Player ID lists for both teams (needed for opposition strength when computing features).
	ids1 := make([]int64, 0, len(pool1))
	for _, p := range pool1 {
		ids1 = append(ids1, p.PlayerID)
	}
	ids2 := make([]int64, 0, len(pool2))
	for _, p := range pool2 {
		ids2 = append(ids2, p.PlayerID)
	}
	weatherOpt := toWeatherOverride(input.Weather)
	// Opposition strength: use the other team's pool (team1 faces team2, team2 faces team1).
	// When input.OppositionPlayerIDs is set, use it as override for the opposition pool (e.g. single-team prediction).
	oppositionIDsTeam1 := ids2
	oppositionIDsTeam2 := ids1
	if len(input.OppositionPlayerIDs) > 0 {
		oppositionIDsTeam1 = input.OppositionPlayerIDs
		oppositionIDsTeam2 = input.OppositionPlayerIDs
	}
	// Features for both teams (each with correct opposition context)
	feats1, err := exportqueries.ComputeFeaturesAtCutoffForFutureMatch(
		ctx,
		cutoff,
		format,
		venueID,
		opp2ID,
		input.SeasonID,
		ids1,
		weatherOpt,
		oppositionIDsTeam1,
	)
	if err != nil {
		slog.Error("predictteam.PredictTeams team1 features failed", slog.String("team1", team1), slog.Any("err", err))
		return nil, nil, fmt.Errorf("team1 features: %w", err)
	}
	feats2, err := exportqueries.ComputeFeaturesAtCutoffForFutureMatch(
		ctx,
		cutoff,
		format,
		venueID,
		opp1ID,
		input.SeasonID,
		ids2,
		weatherOpt,
		oppositionIDsTeam2,
	)
	if err != nil {
		slog.Error("predictteam.PredictTeams team2 features failed", slog.String("team2", team2), slog.Any("err", err))
		return nil, nil, fmt.Errorf("team2 features: %w", err)
	}

	// Combined prediction with match context for hybrid reconciliation (innings model rescaling)
	allIDs := make([]int64, 0, len(ids1)+len(ids2))
	allIDs = append(allIDs, ids1...)
	allIDs = append(allIDs, ids2...)
	allFeats := make(map[int64]map[string]float64, len(feats1)+len(feats2))
	for pid, m := range feats1 {
		allFeats[pid] = m
	}
	for pid, m := range feats2 {
		allFeats[pid] = m
	}
	venueIDVal := int64(0)
	if venueID != nil {
		venueIDVal = *venueID
	}
	seasonIDVal := int64(0)
	if input.SeasonID != nil {
		seasonIDVal = *input.SeasonID
	}
	matchCtx := &MatchContext{
		Team1PlayerIDs:    ids1,
		Team2PlayerIDs:    ids2,
		VenueID:           venueIDVal,
		SeasonID:          seasonIDVal,
		FormatID:          formatID,
		Team1OppositionID: opp2IDVal,
		Team2OppositionID: opp1IDVal,
	}
	allPreds, err := predictor.PredictPlayers(ctx, cutoff, formatForPrediction, allIDs, allFeats, matchCtx)
	if err != nil {
		slog.Error("predictteam.PredictTeams predict failed", slog.Any("err", err))
		return nil, nil, fmt.Errorf("predict: %w", err)
	}
	preds1 := make(map[int64]PlayerPred)
	preds2 := make(map[int64]PlayerPred)
	for _, pid := range ids1 {
		if p, ok := allPreds[pid]; ok {
			preds1[pid] = p
		}
	}
	for _, pid := range ids2 {
		if p, ok := allPreds[pid]; ok {
			preds2[pid] = p
		}
	}
	if !hasFieldingPredictions(preds1) {
		enrichFieldingFromHistory(ctx, preds1, ids1, cutoff, formatID)
	}
	if !hasFieldingPredictions(preds2) {
		enrichFieldingFromHistory(ctx, preds2, ids2, cutoff, formatID)
	}

	// Build teamselect pool and select (format-aware normalization)
	cfg := config.Load()
	tsPool1 := buildTeamSelectPool(pool1, preds1, format, cfg)
	tsPool2 := buildTeamSelectPool(pool2, preds2, format, cfg)

	teamSize := config.DefaultTeamSize
	if cfg != nil && cfg.Predictor.TeamSize > 0 {
		teamSize = cfg.Predictor.TeamSize
	}
	batW, bowlW, fieldW, keeperW := config.EffectiveScoreWeightsForFormat(cfg, format)
	weights := teamselect.ScoreWeights{Bat: batW, Bowl: bowlW, Field: fieldW, KeeperBonus: keeperW}
	constraints := teamselect.Constraints{
		Size:          teamSize,
		MinBowlers:    input.MinBowlers,
		RequireKeeper: input.RequireKeeper,
	}
	useOptimizer := cfg != nil && cfg.Selection.UseOptimizer
	sel1, err := selectTeam(tsPool1, weights, constraints, useOptimizer)
	if err != nil {
		slog.Error("predictteam.PredictTeams team1 select failed", slog.String("team1", team1), slog.Any("err", err))
		return nil, nil, fmt.Errorf("team1 select: %w", err)
	}
	sel2, err := selectTeam(tsPool2, weights, constraints, useOptimizer)
	if err != nil {
		slog.Error("predictteam.PredictTeams team2 select failed", slog.String("team2", team2), slog.Any("err", err))
		return nil, nil, fmt.Errorf("team2 select: %w", err)
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

	// Predicted scorecard summary: innings totals and winner from selected XI predictions.
	extras1, extras2 := getExtrasForMatch(ctx, formatID, venueID)
	summary := ComputeScorecardSummary(result.Team1, result.Team2, extras1, extras2, team1, team2)
	// When win model is available, use it for winner and rescale individual predictions so team totals match win probability.
	p, err := getMatchWinProbability(
		ctx,
		predictor,
		format,
		formatID,
		venueIDVal,
		opp1IDVal,
		opp2IDVal,
		input.Weather,
		nameToID1,
		nameToID2,
		sel1,
		sel2,
		allFeats,
	)
	if err == nil {
		summary.Team1WinProbability = p
		if p >= 0.5 {
			summary.PredictedWinner = team1
		} else {
			summary.PredictedWinner = team2
		}
		rescaleTeamPredictionsToWinProbability(result.Team1, result.Team2, extras1, extras2, p)
		// Recompute summary from rescaled runs
		var runs1, runs2 float64
		for _, p := range result.Team1 {
			runs1 += p.Runs
		}
		for _, p := range result.Team2 {
			runs2 += p.Runs
		}
		summary.Innings1Total = runs1 + extras1
		summary.Innings2Total = runs2 + extras2
	} else if !errors.Is(err, sql.ErrNoRows) { // ErrNoRows is expected if win model is not loaded.
		slog.WarnContext(ctx, "failed to get match win probability", slog.Any("err", err))
	}
	result.ScorecardSummary = &summary

	// Optionally call generate-match for reconciled scorecard (and/or to return both for comparison).
	if (input.UseReconciledScorecard || input.IncludeBothScorecards) && reconciledGen != nil {
		selectedIDs := make([]int64, 0, len(sel1)+len(sel2))
		for _, p := range sel1 {
			if id, ok := nameToID1[p.Name]; ok {
				selectedIDs = append(selectedIDs, id)
			}
		}
		for _, p := range sel2 {
			if id, ok := nameToID2[p.Name]; ok {
				selectedIDs = append(selectedIDs, id)
			}
		}
		featuresForSelected := make(map[int64]map[string]float64, len(selectedIDs))
		for _, pid := range selectedIDs {
			featuresForSelected[pid] = allFeats[pid] // may be nil/empty; ML accepts missing features
		}
		reconciledPlayers, in1, in2, winProb, _, errGen := reconciledGen(ctx, cutoff, formatForPrediction, selectedIDs, featuresForSelected, true, matchCtx)
		if errGen == nil {
			reconciledSummary := ScorecardSummary{
				Innings1Total:       in1,
				Innings2Total:       in2,
				Team1WinProbability: winProb,
			}
			if winProb >= 0.5 {
				reconciledSummary.PredictedWinner = team1
			} else {
				reconciledSummary.PredictedWinner = team2
			}
			result.ScorecardSummaryReconciled = &reconciledSummary
			if input.UseReconciledScorecard {
				result.ScorecardSummary = &reconciledSummary
				// Overwrite per-player stats with reconciled values so the response reflects the reconciled scorecard.
				for i := range result.Team1 {
					if pr, ok := reconciledPlayers[result.Team1[i].PlayerID]; ok {
						result.Team1[i].Runs = pr.Runs
						result.Team1[i].Wickets = pr.Wickets
						result.Team1[i].Economy = pr.Economy
						result.Team1[i].Catches = pr.Catches
						result.Team1[i].RunOuts = pr.RunOuts
					}
				}
				for i := range result.Team2 {
					if pr, ok := reconciledPlayers[result.Team2[i].PlayerID]; ok {
						result.Team2[i].Runs = pr.Runs
						result.Team2[i].Wickets = pr.Wickets
						result.Team2[i].Economy = pr.Economy
						result.Team2[i].Catches = pr.Catches
						result.Team2[i].RunOuts = pr.RunOuts
					}
				}
			}
		} else {
			slog.WarnContext(ctx, "reconciled generate-match failed, skipping reconciled scorecard", slog.Any("err", errGen))
		}
	}

	mid := &predictIntermediates{
		Pool1:    pool1,
		Pool2:    pool2,
		Preds1:   preds1,
		Preds2:   preds2,
		FormatID: formatID,
		VenueID:  venueID,
		Extras1:  extras1,
		Extras2:  extras2,
		Team1:    team1,
		Team2:    team2,
	}
	return result, mid, nil
}

// PredictTeams runs the full pipeline: pool, features, ML predict, team select.
// reconciledGen is optional; when non-nil and input.UseReconciledScorecard or input.IncludeBothScorecards is set,
// it is used to fetch a reconciled scorecard (and optionally both for comparison).
func PredictTeams(ctx context.Context, input Input, predictor MLPredictor, reconciledGen GenerateMatchFunc) (*Result, error) {
	result, _, err := predictTeamsWithIntermediates(ctx, input, predictor, reconciledGen)
	return result, err
}

// ComputeScorecardSummary builds the predicted scorecard summary from two selected XIs and their
// predictions. Innings 1 = team1 batting (sum of runs) + extras1; Innings 2 = team2 batting + extras2.
// PredictedWinner is team1Code or team2Code by higher total, or "" if tied.
func ComputeScorecardSummary(
	team1, team2 []SelectedPlayer,
	extrasInnings1, extrasInnings2 float64,
	team1Code, team2Code string,
) ScorecardSummary {
	var runs1, runs2 float64
	for _, p := range team1 {
		runs1 += p.Runs
	}
	for _, p := range team2 {
		runs2 += p.Runs
	}
	innings1Total := runs1 + extrasInnings1
	innings2Total := runs2 + extrasInnings2
	predictedWinner := ""
	if innings1Total > innings2Total {
		predictedWinner = team1Code
	} else if innings2Total > innings1Total {
		predictedWinner = team2Code
	}
	return ScorecardSummary{
		Innings1Total:   innings1Total,
		Innings2Total:   innings2Total,
		PredictedWinner: predictedWinner,
		ExtrasInnings1:  extrasInnings1,
		ExtrasInnings2:  extrasInnings2,
	}
}

// rescaleTeamPredictionsToWinProbability rescales each player's Runs (and Wickets, Economy) so that
// team totals match win probability p: innings1 = total_innings*p, innings2 = total_innings*(1-p).
func rescaleTeamPredictionsToWinProbability(
	team1, team2 []SelectedPlayer,
	extras1, extras2 float64,
	p float64,
) {
	var runs1, runs2 float64
	for _, p := range team1 {
		runs1 += p.Runs
	}
	for _, p := range team2 {
		runs2 += p.Runs
	}
	totalInnings := runs1 + extras1 + runs2 + extras2
	if totalInnings <= 0 {
		return
	}
	targetRuns1 := totalInnings*p - extras1
	targetRuns2 := totalInnings*(1-p) - extras2
	factor1, factor2 := 1.0, 1.0
	if runs1 > 0 {
		factor1 = targetRuns1 / runs1
	}
	if runs2 > 0 {
		factor2 = targetRuns2 / runs2
	}
	for i := range team1 {
		team1[i].Runs *= factor1
	}
	for i := range team2 {
		team2[i].Runs *= factor2
	}
}

// getMatchWinProbability builds match-level win features from selected XIs and allFeats, calls the win model, and returns team1 win probability.
// Returns error when the win model is not loaded or the request fails; caller then keeps winner-from-totals.
func getMatchWinProbability(
	ctx context.Context,
	predictor MLPredictor,
	format string,
	formatID, venueIDVal, opp1IDVal, opp2IDVal int64,
	weather *WeatherInput,
	nameToID1, nameToID2 map[string]int64,
	sel1, sel2 []teamselect.Player,
	allFeats map[int64]map[string]float64,
) (float64, error) {
	sumBatConsistency := func(ids []int64) float64 {
		s := 0.0
		for _, pid := range ids {
			if m := allFeats[pid]; m != nil {
				s += m["batting_consistency"]
			}
		}
		return s
	}
	sumBowlConsistency := func(ids []int64) float64 {
		s := 0.0
		for _, pid := range ids {
			if m := allFeats[pid]; m != nil {
				s += m["bowling_consistency"]
			}
		}
		return s
	}
	sumBatForm := func(ids []int64) float64 {
		s := 0.0
		for _, pid := range ids {
			if m := allFeats[pid]; m != nil {
				s += m["batting_form"]
			}
		}
		return s
	}
	sumBowlForm := func(ids []int64) float64 {
		s := 0.0
		for _, pid := range ids {
			if m := allFeats[pid]; m != nil {
				s += m["bowling_form"]
			}
		}
		return s
	}
	ids1 := make([]int64, 0, len(sel1))
	for _, p := range sel1 {
		if id, ok := nameToID1[p.Name]; ok {
			ids1 = append(ids1, id)
		}
	}
	ids2 := make([]int64, 0, len(sel2))
	for _, p := range sel2 {
		if id, ok := nameToID2[p.Name]; ok {
			ids2 = append(ids2, id)
		}
	}
	temp, wind, rain, humidity, cloud, pressure := 0, 0, 0, 0, 0, 0
	if weather != nil {
		temp = int(weather.Temp)
		wind = int(weather.Wind)
		rain = int(weather.Rain)
		humidity = int(weather.Humidity)
		cloud = int(weather.Cloud)
		pressure = int(weather.Pressure)
	}
	w := WinFeatures{
		FormatID:                int(formatID),
		VenueID:                 int(venueIDVal),
		Team1OppositionID:       int(opp2IDVal), // team1 bats first, faces team2
		Team2OppositionID:       int(opp1IDVal),
		TossWinnerOppositionID:  0,
		Temp:                    temp,
		Wind:                    wind,
		Rain:                    rain,
		Humidity:                humidity,
		Cloud:                   cloud,
		Pressure:                pressure,
		Viscosity:               0,
		Team1BatConsistencySum:  sumBatConsistency(ids1),
		Team1BowlConsistencySum: sumBowlConsistency(ids1),
		Team2BatConsistencySum:  sumBatConsistency(ids2),
		Team2BowlConsistencySum: sumBowlConsistency(ids2),
		Team1BatFormSum:         sumBatForm(ids1),
		Team1BowlFormSum:        sumBowlForm(ids1),
		Team2BatFormSum:         sumBatForm(ids2),
		Team2BowlFormSum:        sumBowlForm(ids2),
		Format:                  strings.TrimSpace(strings.ToUpper(format)),
	}
	return predictor.PredictMatchWin(ctx, w)
}

// getExtrasForMatch returns predicted extras per innings (same for both innings from format/venue average).
// Uses db.GetAverageExtrasForFormat; average is total per match so we split in half for each innings.
func getExtrasForMatch(ctx context.Context, formatID int64, venueID *int64) (extras1, extras2 float64) {
	avg, err := db.GetAverageExtrasForFormat(ctx, formatID, venueID)
	if err != nil || avg <= 0 {
		return 0, 0
	}
	half := avg / 2
	return half, half
}

// selectTeam returns the selected XI using either constrained optimization or greedy selection.
func selectTeam(
	pool []teamselect.Player,
	weights teamselect.ScoreWeights,
	constraints teamselect.Constraints,
	useOptimizer bool,
) ([]teamselect.Player, error) {
	if useOptimizer {
		return teamselect.SelectOptimized(pool, weights, constraints)
	}
	return teamselect.Select(pool, weights, constraints)
}

func buildTeamSelectPool(
	pool []db.PlayerPoolRow,
	preds map[int64]PlayerPred,
	format string,
	cfg *config.Config,
) []teamselect.Player {
	batDiv, wicketDiv, econBase, fieldDiv := config.EffectiveScoreNormParams(cfg, format)
	out := make([]teamselect.Player, 0, len(pool))
	for _, p := range pool {
		pr := preds[p.PlayerID]
		isBowler := p.BowlingConsistency.Valid && p.BowlingConsistency.Float64 > 0
		batScore := teamselect.NormalizeBatScore(pr.Runs, batDiv)
		bowlScore := 0.0
		if isBowler {
			bowlScore = teamselect.NormalizeBowlScore(pr.Wickets, pr.Economy, wicketDiv, econBase)
		}
		fieldScore := teamselect.NormalizeFieldScore(pr.Catches, pr.RunOuts, fieldDiv)
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

func toWeatherOverride(w *WeatherInput) *exportqueries.WeatherOverride {
	if w == nil {
		return nil
	}
	return &exportqueries.WeatherOverride{
		Temp:     w.Temp,
		Humidity: w.Humidity,
		Wind:     w.Wind,
		Rain:     w.Rain,
		Cloud:    w.Cloud,
		Pressure: w.Pressure,
	}
}

// hasFieldingPredictions returns true if any prediction has non-zero catches or run_outs,
// i.e. the ML service returned fielding model output. When true, do not overwrite with enrichFieldingFromHistory.
func hasFieldingPredictions(preds map[int64]PlayerPred) bool {
	for _, p := range preds {
		if p.Catches > 0 || p.RunOuts > 0 {
			return true
		}
	}
	return false
}

// enrichFieldingFromHistory overwrites Catches and RunOuts in preds using historical
// fielding form (EWM of involvements). Used only when the ML service did not return
// fielding predictions (no fielding model loaded for this format).
// EWM alpha and form-to-catches ratio are read from config (features.fielding_enrich).
func enrichFieldingFromHistory(
	ctx context.Context,
	preds map[int64]PlayerPred,
	playerIDs []int64,
	cutoff time.Time,
	formatID int64,
) {
	ewmAlpha := config.DefaultFieldingEWMAlpha
	catchesRatio := config.DefaultFieldingFormToCatchesRatio
	if cfg := config.Load(); cfg != nil {
		if cfg.Features.FieldingEnrich.EWMAlpha > 0 && cfg.Features.FieldingEnrich.EWMAlpha <= 1 {
			ewmAlpha = cfg.Features.FieldingEnrich.EWMAlpha
		}
		if cfg.Features.FieldingEnrich.FormToCatchesRatio > 0 && cfg.Features.FieldingEnrich.FormToCatchesRatio <= 1 {
			catchesRatio = cfg.Features.FieldingEnrich.FormToCatchesRatio
		}
	}
	keys := make([]db.FieldingHistKey, 0, len(playerIDs))
	for _, pid := range playerIDs {
		keys = append(keys, db.FieldingHistKey{P: pid, T: cutoff, F: formatID})
	}
	bulkRes, err := db.ListFieldingBeforeBulk(ctx, keys)
	if err != nil {
		return
	}
	for _, k := range keys {
		hist := bulkRes[k]
		if len(hist) == 0 {
			continue
		}
		inn := make([]features.Innings, 0, len(hist))
		for _, iv := range hist {
			if iv.MatchDate.Before(cutoff) {
				inn = append(inn, features.Innings{Date: iv.MatchDate, Value: iv.Value})
			}
		}
		inn = features.SortAndClip(inn, cutoff)
		if len(inn) == 0 {
			continue
		}
		form, _ := features.EWM(inn, ewmAlpha)
		catches := form * catchesRatio
		runOuts := form * (1 - catchesRatio)
		pid := k.P
		if p, ok := preds[pid]; ok {
			p.Catches = math.Max(0, catches)
			p.RunOuts = math.Max(0, runOuts)
			preds[pid] = p
		}
	}
}
