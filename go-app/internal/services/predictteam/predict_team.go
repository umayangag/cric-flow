// Package predictteam implements future-match team selection: pick best 11 for each team
// using ML predictions (batting, bowling, fielding) with venue, opposition, and weather context.
package predictteam

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/config"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db/exportqueries"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/features"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/teamselect"
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
	MinBowlers          int           `json:"min_bowlers,omitempty"`           // default 5
	RequireKeeper       bool          `json:"require_keeper,omitempty"`        // default true
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
		return nil, err
	}
	cutoff := input.MatchDate.Truncate(24 * time.Hour)

	formatID, fmtErr := db.GetGlobalCache().GetFormatID(ctx, format)
	if fmtErr != nil {
		slog.Error("predictteam.PredictTeams resolve format failed", slog.String("format", format), slog.Any("err", fmtErr))
		return nil, fmt.Errorf("resolve format: %w", fmtErr)
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

	// Player pools
	pool1, err := db.ListPlayerPoolByTeam(ctx, format, team1, cutoff, input.ExtraTeam1)
	if err != nil {
		slog.Error("predictteam.PredictTeams team1 pool failed", slog.String("team1", team1), slog.Any("err", err))
		return nil, fmt.Errorf("team1 pool: %w", err)
	}
	pool2, err := db.ListPlayerPoolByTeam(ctx, format, team2, cutoff, input.ExtraTeam2)
	if err != nil {
		slog.Error("predictteam.PredictTeams team2 pool failed", slog.String("team2", team2), slog.Any("err", err))
		return nil, fmt.Errorf("team2 pool: %w", err)
	}
	if len(pool1) < 11 {
		err := fmt.Errorf("team1 has only %d players, need at least 11", len(pool1))
		slog.Error("predictteam.PredictTeams pool size", slog.String("team1", team1), slog.Int("len", len(pool1)), slog.Any("err", err))
		return nil, err
	}
	if len(pool2) < 11 {
		err := fmt.Errorf("team2 has only %d players, need at least 11", len(pool2))
		slog.Error("predictteam.PredictTeams pool size", slog.String("team2", team2), slog.Int("len", len(pool2)), slog.Any("err", err))
		return nil, err
	}

	// Features and predictions for team1 (opposition = team2)
	ids1 := make([]int64, 0, len(pool1))
	for _, p := range pool1 {
		ids1 = append(ids1, p.PlayerID)
	}
	weatherOpt := toWeatherOverride(input.Weather)
	feats1, err := exportqueries.ComputeFeaturesAtCutoffForFutureMatch(
		ctx,
		cutoff,
		format,
		venueID,
		opp2ID,
		input.SeasonID,
		ids1,
		weatherOpt,
	)
	if err != nil {
		slog.Error("predictteam.PredictTeams team1 features failed", slog.String("team1", team1), slog.Any("err", err))
		return nil, fmt.Errorf("team1 features: %w", err)
	}
	preds1, err := predictor.PredictPlayers(ctx, cutoff, format, ids1, feats1)
	if err != nil {
		slog.Error("predictteam.PredictTeams team1 predict failed", slog.String("team1", team1), slog.Any("err", err))
		return nil, fmt.Errorf("team1 predict: %w", err)
	}
	// Use ML fielding when the service returned predictions; otherwise fall back to historical EWM.
	if !hasFieldingPredictions(preds1) {
		enrichFieldingFromHistory(ctx, preds1, ids1, cutoff, formatID)
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
		weatherOpt,
	)
	if err != nil {
		slog.Error("predictteam.PredictTeams team2 features failed", slog.String("team2", team2), slog.Any("err", err))
		return nil, fmt.Errorf("team2 features: %w", err)
	}
	preds2, err := predictor.PredictPlayers(ctx, cutoff, format, ids2, feats2)
	if err != nil {
		slog.Error("predictteam.PredictTeams team2 predict failed", slog.String("team2", team2), slog.Any("err", err))
		return nil, fmt.Errorf("team2 predict: %w", err)
	}
	if !hasFieldingPredictions(preds2) {
		enrichFieldingFromHistory(ctx, preds2, ids2, cutoff, formatID)
	}

	// Build teamselect pool and select
	tsPool1 := buildTeamSelectPool(pool1, preds1)
	tsPool2 := buildTeamSelectPool(pool2, preds2)

	teamSize := config.DefaultTeamSize
	if cfg := config.Load(); cfg != nil && cfg.Predictor.TeamSize > 0 {
		teamSize = cfg.Predictor.TeamSize
	}
	batW, bowlW, fieldW, keeperW := config.EffectiveScoreWeights(config.Load())
	weights := teamselect.ScoreWeights{Bat: batW, Bowl: bowlW, Field: fieldW, KeeperBonus: keeperW}
	sel1, err := teamselect.Select(tsPool1, weights, teamselect.Constraints{
		Size:          teamSize,
		MinBowlers:    input.MinBowlers,
		RequireKeeper: input.RequireKeeper,
	})
	if err != nil {
		slog.Error("predictteam.PredictTeams team1 select failed", slog.String("team1", team1), slog.Any("err", err))
		return nil, fmt.Errorf("team1 select: %w", err)
	}
	sel2, err := teamselect.Select(tsPool2, weights, teamselect.Constraints{
		Size:          teamSize,
		MinBowlers:    input.MinBowlers,
		RequireKeeper: input.RequireKeeper,
	})
	if err != nil {
		slog.Error("predictteam.PredictTeams team2 select failed", slog.String("team2", team2), slog.Any("err", err))
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

func normalizeFieldScore(catches, runOuts float64) float64 {
	// Catches + run_outs, typical max ~3-4 per match
	return math.Min(1, (catches+runOuts*1.5)/5)
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
