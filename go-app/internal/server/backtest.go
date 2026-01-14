package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// types and seams moved to dedicated files for clarity

// --- Accuracy Trend: Helpers & Refactor Support ---

// Parameter parsing errors (used to keep handler responses identical)
var (
	errInvalidStartDate = errors.New("invalid start_date")
	errInvalidEndDate   = errors.New("invalid end_date")
	errEndBeforeStart   = errors.New("end_date before start_date")
)

type accuracyTrendParams struct {
	Format string
	Team1  string
	Team2  string
	Order  string
	Limit  int
	Cache  string
	// Metrics selection
	IncludePlayer bool
	IncludeTeam   bool
	Start         time.Time
	End           time.Time
	HasStart      bool
	HasEnd        bool
	RawStart      string
	RawEnd        string
}

func parseBacktestAccuracyTrendParams(r *http.Request) (accuracyTrendParams, error) {
	q := r.URL.Query()
	out := accuracyTrendParams{
		Format:   strings.TrimSpace(q.Get("format")),
		Team1:    strings.TrimSpace(q.Get("team1")),
		Team2:    strings.TrimSpace(q.Get("team2")),
		Order:    strings.TrimSpace(q.Get("order")),
		Cache:    strings.TrimSpace(q.Get("cache")),
		RawStart: strings.TrimSpace(q.Get("start_date")),
		RawEnd:   strings.TrimSpace(q.Get("end_date")),
	}
	if out.Order == "" {
		out.Order = "asc"
	}
	switch strings.ToLower(out.Cache) {
	case "", "readwrite":
		out.Cache = "readwrite"
	case "read":
		out.Cache = "read"
	case "off":
		out.Cache = "off"
	default:
		out.Cache = "readwrite"
	}
	if s := strings.TrimSpace(q.Get("limit")); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			out.Limit = v
		}
	}
	// Enforce default limit and max cap
	if out.Limit <= 0 {
		out.Limit = 100
	}
	if out.Limit > 500 {
		out.Limit = 500
	}
	// Parse dates in YYYY-MM-DD
	if out.RawStart != "" {
		t, err := time.Parse("2006-01-02", out.RawStart)
		if err != nil {
			return accuracyTrendParams{}, errInvalidStartDate
		}
		out.Start, out.HasStart = t, true
	}
	if out.RawEnd != "" {
		t, err := time.Parse("2006-01-02", out.RawEnd)
		if err != nil {
			return accuracyTrendParams{}, errInvalidEndDate
		}
		out.End, out.HasEnd = t, true
	}
	if out.HasStart && out.HasEnd && out.End.Before(out.Start) {
		return accuracyTrendParams{}, errEndBeforeStart
	}

	// Parse metrics selection
	metrics := strings.TrimSpace(q.Get("metrics"))
	if metrics == "" {
		out.IncludePlayer = true
		out.IncludeTeam = true
	} else {
		parts := strings.Split(metrics, ",")
		for _, p := range parts {
			switch strings.ToLower(strings.TrimSpace(p)) {
			case "player":
				out.IncludePlayer = true
			case "team":
				out.IncludeTeam = true
			}
		}
		// If user passed unknown tokens, default to both to be safe
		if !out.IncludePlayer && !out.IncludeTeam {
			out.IncludePlayer = true
			out.IncludeTeam = true
		}
	}
	return out, nil
}

// Cache seams for match-level aggregates (overridable in tests)
var (
	getMatchPredictionAggregatesFunc    = db.GetMatchPredictionAggregates
	upsertMatchPredictionAggregatesFunc = db.UpsertMatchPredictionAggregates
)

// computeAccuracyTrendMetrics calculates metrics for a single match candidate.
// It mirrors the previous inline logic to avoid behavior changes.
func computeAccuracyTrendMetrics(
	ctx context.Context,
	m backtestCandidate,
	cacheMode string,
	includePlayer, includeTeam bool,
) map[string]float64 {
	metrics := map[string]float64{}

	// Determine cutoff (match date)
	cutoff, cerr := getBacktestMatchDateFunc(ctx, m.MatchID)
	if cerr != nil || cutoff.IsZero() {
		if t, err := time.Parse(time.RFC3339, m.Date); err == nil {
			cutoff = t
		}
	}
	if cutoff.IsZero() {
		return metrics
	}

	// Player-level MAE on runs (optional)
	if includePlayer {
		if squad, err := getBacktestSquadPlayerIDsFunc(ctx, m.MatchID, cutoff, m.Format); err == nil && len(squad) > 0 {
			preds, err1 := mlBacktestPredictFunc(ctx, cutoff, squad)
			acts, err2 := getBacktestPlayerActualsForMatchFunc(ctx, m.MatchID)
			if err1 == nil && err2 == nil {
				var totalAbs, cnt float64
				for _, pid := range squad {
					pPred, okp := preds[pid]
					pAct, oka := acts[pid]
					if !okp || !oka {
						continue
					}
					totalAbs += math.Abs(pPred.Runs - pAct.Runs)
					cnt++
				}
				if cnt > 0 {
					metrics["player_runs_mae"] = totalAbs / cnt
				}
			}
		}
	}

	// Match/team aggregates with optional cache (optional)
	if includeTeam && m.Team1 != "" && m.Team2 != "" {
		var predAgg matchAggregates
		var havePred bool

		if cacheMode == "read" || cacheMode == "readwrite" {
			if rec, err := getMatchPredictionAggregatesFunc(ctx, m.MatchID); err == nil {
				predAgg = matchAggregates{
					Runs:           rec.PredictedTotalRuns.Float64,
					WinnerTeamCode: rec.PredictedWinnerCode.String,
				}
				havePred = true
			}
		}

		if !havePred {
			if p, modelVersion, err := mlBacktestPredictMatchAggregatesFunc(ctx, cutoff, [2]string{m.Team1, m.Team2}); err == nil {
				predAgg = p
				havePred = true
				if cacheMode == "readwrite" {
					if err := upsertMatchPredictionAggregatesFunc(ctx, db.MatchPredictionAggregates{
						MatchID:             m.MatchID,
						Format:              m.Format,
						Team1Code:           m.Team1,
						Team2Code:           m.Team2,
						PredictedWinnerCode: sqlNullString(predAgg.WinnerTeamCode),
						PredictedTotalRuns:  sqlNullFloat64(predAgg.Runs),
						ModelVersion:        sqlNullString(modelVersion),
						CutoffAt:            cutoff,
					}); err != nil {
						// Failing to cache is not critical for the request, but should be monitored
						slog.Warn(
							"failed to upsert match prediction aggregates cache",
							slog.Any("err", err),
							slog.Int64("match_id", m.MatchID),
							slog.String("format", m.Format),
							slog.String("team1", m.Team1),
							slog.String("team2", m.Team2),
							slog.Time("cutoff_at", cutoff),
						)
					}
				}
			}
		}

		if havePred {
			if actAgg, errA := getBacktestMatchAggregatesActualsFunc(ctx, m.MatchID); errA == nil {
				if actAgg.Runs != 0 || predAgg.Runs != 0 {
					metrics["team_runs_mae"] = math.Abs(predAgg.Runs - actAgg.Runs)
				}
				if actAgg.WinnerTeamCode != "" && predAgg.WinnerTeamCode != "" {
					if strings.EqualFold(actAgg.WinnerTeamCode, predAgg.WinnerTeamCode) {
						metrics["team_winner_accuracy"] = 1.0
					} else {
						metrics["team_winner_accuracy"] = 0.0
					}
				}
			}
		}
	}

	return metrics
}

// computeAccuracyTrendSummaryAndProgressive builds the summary and progressive rows
// from the list of result items. For each metric, averages are computed using the
// count of matches where the metric is actually present as the denominator.
func computeAccuracyTrendSummaryAndProgressive(items []accuracyTrendItem) (map[string]float64, []map[string]float64) {
	var (
		sumPlayerMAE, nPlayerMAE     float64
		sumTeamRunsMAE, nTeamRunsMAE float64
		sumWinnerAcc, nWinnerAcc     float64
	)
	nMatches := float64(len(items))

	// Summary accumulators
	for _, it := range items {
		if v, ok := it.Metrics["player_runs_mae"]; ok {
			sumPlayerMAE += v
			nPlayerMAE++
		}
		if v, ok := it.Metrics["team_runs_mae"]; ok {
			sumTeamRunsMAE += v
			nTeamRunsMAE++
		}
		if v, ok := it.Metrics["team_winner_accuracy"]; ok {
			sumWinnerAcc += v
			nWinnerAcc++
		}
	}

	summary := map[string]float64{"n": nMatches}
	if nPlayerMAE > 0 {
		summary["player_runs_mae_avg"] = sumPlayerMAE / nPlayerMAE
	}
	if nTeamRunsMAE > 0 {
		summary["team_runs_mae_avg"] = sumTeamRunsMAE / nTeamRunsMAE
	}
	if nWinnerAcc > 0 {
		summary["team_winner_accuracy_avg"] = sumWinnerAcc / nWinnerAcc
	}

	// Progressive calculations using per-metric present counts
	progressive := make([]map[string]float64, 0, len(items))
	var (
		psPlayer, pnPlayer     float64
		psTeamRuns, pnTeamRuns float64
		psWinner, pnWinner     float64
	)
	for i, it := range items {
		n := float64(i + 1)
		if v, ok := it.Metrics["player_runs_mae"]; ok {
			psPlayer += v
			pnPlayer++
		}
		if v, ok := it.Metrics["team_runs_mae"]; ok {
			psTeamRuns += v
			pnTeamRuns++
		}
		if v, ok := it.Metrics["team_winner_accuracy"]; ok {
			psWinner += v
			pnWinner++
		}
		row := map[string]float64{"n": n}
		if pnPlayer > 0 {
			row["player_runs_mae_avg"] = psPlayer / pnPlayer
		}
		if pnTeamRuns > 0 {
			row["team_runs_mae_avg"] = psTeamRuns / pnTeamRuns
		}
		if pnWinner > 0 {
			row["team_winner_accuracy_avg"] = psWinner / pnWinner
		}
		progressive = append(progressive, row)
	}

	return summary, progressive
}

// helpers to construct sql nullable types
func sqlNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func sqlNullFloat64(v float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: v, Valid: true}
}

// handlers moved to backtest_handlers.go

// handlers moved to backtest_handlers.go

func nullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// Wire default implementations for evaluate-mode seams to DB repos where available.
func init() {
	getBacktestMatchDateFunc = func(ctx context.Context, matchID int64) (time.Time, error) {
		d, err := db.GetMatchDate(ctx, matchID)
		if err != nil {
			return time.Time{}, err
		}
		if d == nil {
			return time.Time{}, sql.ErrNoRows
		}
		return *d, nil
	}
	// Minimal default for squad player IDs: read from player_match for the given match_id.
	getBacktestSquadPlayerIDsFunc = func(ctx context.Context, matchID int64, _ time.Time, _ string) ([]int64, error) {
		if db.Pool == nil {
			return nil, errors.New("db pool not initialized")
		}
		rows, err := db.Pool.Query(
			ctx,
			`SELECT DISTINCT player_id FROM player_match WHERE match_id = $1 ORDER BY player_id ASC`,
			matchID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		ids := make([]int64, 0, 22)
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return ids, nil
	}
	// Actuals for match: optimized single-query LEFT JOIN across batting, bowling, fielding
	getBacktestPlayerActualsForMatchFunc = func(ctx context.Context, matchID int64) (map[int64]playerActuals, error) {
		if db.Pool == nil {
			return nil, errors.New("db pool not initialized")
		}
		// Unified query to reduce DB round-trips: get all player actuals with LEFT JOINs
		rows, err := db.Pool.Query(
			ctx,
			`
         SELECT
             pm.player_id,
             COALESCE(bd.runs, 0) AS runs,
             COALESCE(bw.wickets, 0) AS wickets,
             COALESCE(bw.econ, 0) AS econ,
             COALESCE(fd.catches, 0) AS catches,
             COALESCE(fd.run_outs, 0) AS run_outs
         FROM player_match pm
         LEFT JOIN batting_data bd ON bd.match_id = pm.match_id AND bd.player_id = pm.player_id
         LEFT JOIN bowling_data bw ON bw.match_id = pm.match_id AND bw.player_id = pm.player_id
         LEFT JOIN fielding_data fd ON fd.match_id = pm.match_id AND fd.player_id = pm.player_id
         WHERE pm.match_id = $1
         ORDER BY pm.player_id ASC
         `,
			matchID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make(map[int64]playerActuals, 22)
		for rows.Next() {
			var (
				pid                                   int64
				runs, wickets, econ, catches, runOuts float64
			)
			if err := rows.Scan(&pid, &runs, &wickets, &econ, &catches, &runOuts); err != nil {
				return nil, err
			}
			out[pid] = playerActuals{
				Runs:    runs,
				Wickets: wickets,
				Economy: econ,
				Catches: catches,
				RunOuts: runOuts,
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return out, nil
	}
	// Leave features/ML seams as placeholders; tests override them.
	// Wire default ML and aggregates seams to concrete clients/repos where available.
	// These can be overridden in tests.
	mlClient := NewBacktestMLClient()
	mlBacktestPredictFunc = func(ctx context.Context, cutoff time.Time, playerIDs []int64) (map[int64]playerPredictions, error) {
		// If mlClient is nil (should not happen), return placeholder error
		if mlClient == nil {
			return nil, errors.New("ml client not initialized")
		}
		return mlClient.predictPlayers(ctx, cutoff, playerIDs)
	}
	mlBacktestPredictMatchAggregatesFunc = func(ctx context.Context, cutoff time.Time, teams [2]string) (matchAggregates, string, error) {
		if mlClient == nil {
			return matchAggregates{}, "", errors.New("ml client not initialized")
		}
		return mlClient.predictMatchAggregates(ctx, cutoff, teams)
	}
	getBacktestMatchAggregatesActualsFunc = func(ctx context.Context, matchID int64) (matchAggregates, error) {
		ma, err := db.GetMatchAggregates(ctx, matchID)
		if err != nil {
			return matchAggregates{}, err
		}
		return matchAggregates{
			Runs:           ma.Runs,
			Wickets:        ma.Wickets,
			Extras:         ma.Extras,
			WinnerTeamCode: ma.WinnerTeamCode,
		}, nil
	}
}
