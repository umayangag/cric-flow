package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/backtest"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// --- Helpers: thin delegations to services/backtest ---

func chooseBacktestMode(modeInput, matchID string) string {
	return backtest.ChooseBacktestMode(modeInput, matchID)
}

func computeR2(totalSquaredError float64, actuals []float64) float64 {
	return backtest.ComputeR2(totalSquaredError, actuals)
}

func winnerAccuracy(predWinner, actualWinner string) float64 {
	return backtest.WinnerAccuracy(predWinner, actualWinner)
}

func buildPredictedScorecard(
	actual *db.MatchScorecard,
	preds map[int64]playerPredictions,
	playerTeams map[int64]string,
) *db.MatchScorecard {
	return backtest.BuildPredictedScorecard(actual, preds, playerTeams)
}

func intPtr(n int) *int { return &n }

func float32Ptr(f float32) *float32 { return &f }

func computePlayerResultsAndMetrics(
	squad []int64,
	preds map[int64]playerPredictions,
	actuals map[int64]playerActuals,
) ([]BacktestPlayerResult, map[string]float64) {
	return backtest.ComputePlayerResultsAndMetrics(squad, preds, actuals)
}

func formatFromFilters(filters map[string]any) string {
	return backtest.FormatFromFilters(filters)
}

func buildBacktestWinFeatures(
	winCtx *db.MatchWinContext,
	format string,
	playerTeams map[int64]string,
	team1, team2 string,
	features map[int64]map[string]float64,
) predictteam.WinFeatures {
	return backtest.BuildBacktestWinFeatures(winCtx, format, playerTeams, team1, team2, features)
}

func rescaleBacktestPredictionsToWinProbability(
	resp *backtestEvaluateResponse,
	playerTeams map[int64]string,
	team1, team2 string,
	p float64,
) {
	backtest.RescalePredictionsToWinProbability(resp, playerTeams, team1, team2, p)
}

// populateMatchAggregatesAndMetrics fills the response with match-level predicted/actual aggregates
// and associated metrics. When features is non-nil, uses the win model for winner and rescales
// individual predicted runs (and wickets) so team totals match win probability.
func populateMatchAggregatesAndMetrics(
	ctx context.Context,
	resp *backtestEvaluateResponse,
	_ time.Time,
	team1 string,
	team2 string,
	matchID int64,
	features map[int64]map[string]float64,
) {
	playerTeams, err := db.GetMatchPlayerTeams(ctx, matchID)
	if err != nil {
		slog.WarnContext(ctx, "failed to get match player teams", slog.Int64("match_id", matchID), slog.Any("err", err))
		playerTeams = nil
	}
	if playerTeams == nil {
		playerTeams = make(map[int64]string)
	}

	// When win model is available and we have features, get win probability and rescale predictions.
	if len(features) > 0 {
		winCtx, err := db.GetMatchWinContext(ctx, matchID)
		if err != nil {
			slog.WarnContext(
				ctx,
				"failed to get match win context for rescaling",
				slog.Int64("match_id", matchID),
				slog.Any("err", err),
			)
		} else {
			w := buildBacktestWinFeatures(winCtx, formatFromFilters(resp.Filters), playerTeams, team1, team2, features)
			p, err := mlPredictMatchWinFunc(ctx, w)
			if err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					slog.WarnContext(ctx, "win model prediction failed during rescaling", slog.Int64("match_id", matchID), slog.Any("err", err))
				}
			} else if p >= 0 && p <= 1 {
				rescaleBacktestPredictionsToWinProbability(resp, playerTeams, team1, team2, p)
			}
		}
	}

	// Predicted: sum from (possibly rescaled) player predictions; winner from win model or team run totals.
	var predRuns, predWickets float64
	teamRuns := make(map[string]float64)
	for _, p := range resp.Players {
		r := 0.0
		if p.Predicted != nil {
			if v, ok := p.Predicted["runs"]; ok {
				r = v
			}
		}
		predRuns += r
		if t, ok := playerTeams[p.PlayerID]; ok {
			teamRuns[t] += r
		}
		w := 0.0
		if p.Predicted != nil {
			if v, ok := p.Predicted["wickets"]; ok {
				w = v
			}
		}
		predWickets += w
	}
	predWinner := ""
	if teamRuns[team1] > teamRuns[team2] {
		predWinner = team1
	} else if teamRuns[team2] > teamRuns[team1] {
		predWinner = team2
	}

	actAgg, err2 := getBacktestMatchAggregatesActualsFunc(ctx, matchID)
	if err2 != nil {
		return
	}
	// Predicted extras from historical average for this format/venue (no default constant)
	predExtras := 0.0
	if mfc, err := db.GetMatchFeatureContext(ctx, matchID); err != nil {
		slog.WarnContext(
			ctx,
			"failed to get match feature context for extras prediction",
			slog.Int64("match_id", matchID),
			slog.Any("err", err),
		)
	} else {
		if avg, err := db.GetAverageExtrasForFormat(ctx, mfc.FormatID, mfc.VenueID); err != nil {
			slog.WarnContext(ctx, "failed to get average extras for format", slog.Int64("format_id", mfc.FormatID), slog.Any("err", err))
		} else {
			predExtras = avg
		}
	}
	resp.MatchAggregates.Predicted = map[string]any{
		"runs":             predRuns,
		"wickets":          predWickets,
		"extras":           predExtras,
		"winner_team_code": predWinner,
	}
	resp.MatchAggregates.Actual = map[string]any{
		"runs":             actAgg.Runs,
		"wickets":          actAgg.Wickets,
		"extras":           actAgg.Extras,
		"winner_team_code": actAgg.WinnerTeamCode,
	}
	resp.MatchAggregates.Errors = map[string]float64{}
	resp.MatchAggregates.Errors["runs_mae"] = math.Abs(predRuns - actAgg.Runs)
	resp.MatchAggregates.Errors["wickets_mae"] = math.Abs(predWickets - actAgg.Wickets)
	resp.MatchAggregates.Errors["extras_mae"] = math.Abs(predExtras - actAgg.Extras)
	if resp.Metrics == nil {
		resp.Metrics = map[string]float64{}
	}
	resp.Metrics["match_runs_mae"] = resp.MatchAggregates.Errors["runs_mae"]
	resp.Metrics["match_wickets_mae"] = resp.MatchAggregates.Errors["wickets_mae"]
	resp.Metrics["match_extras_mae"] = resp.MatchAggregates.Errors["extras_mae"]
	resp.Metrics["winner_accuracy"] = winnerAccuracy(predWinner, actAgg.WinnerTeamCode)
}
