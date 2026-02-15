package server

import (
	"context"
	"database/sql"
	"time"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	exq "github.com/umayangag/cric-info-scrapers/go-app/internal/db/exportqueries"
)

// testing seam for DB call
var listPlayedByFmtTeams = db.ListPlayedMatchesByFormatAndTeams

// Evaluate-mode seams (to be backed by DB repos; overridden in tests)
var (
	// Returns the match date (cutoff) for the given match id
	getBacktestMatchDateFunc = func(_ context.Context, _ int64) (time.Time, error) {
		// Placeholder: to be implemented via db repo in a later step
		return time.Time{}, sql.ErrNoRows
	}
	// Returns the list of player IDs who actually played the match (XI + subs if available)
	// Requires cutoff and optional format to align with DB query semantics
	getBacktestSquadPlayerIDsFunc = func(_ context.Context, _ int64, _ time.Time, _ string) ([]int64, error) {
		return nil, sql.ErrNoRows
	}
	// Returns actuals for players in the match, keyed by player id; minimal target: runs
	getBacktestPlayerActualsForMatchFunc = func(_ context.Context, _ int64) (map[int64]playerActuals, error) {
		return nil, sql.ErrNoRows
	}
	// Features at cutoff: precomputed only (no averages). When matchID > 0 uses match context; when 0 uses format only (venue/opposition 0).
	getBacktestFeaturesAtCutoffFunc = func(ctx context.Context, cutoff time.Time, playerIDs []int64, matchID int64, format string) (map[int64]map[string]float64, error) {
		if matchID > 0 {
			return exq.ComputeFeaturesAtCutoffForMatch(ctx, matchID, cutoff, playerIDs)
		}
		return exq.ComputeFeaturesAtCutoffNoMatch(ctx, cutoff, format, playerIDs)
	}
	// ML seam for backtest: given cutoff, format, player ids, and optional features, return predicted targets per player
	mlBacktestPredictFunc = func(_ context.Context, _ time.Time, _ string, _ []int64, _ map[int64]map[string]float64) (map[int64]playerPredictions, error) {
		return nil, sql.ErrNoRows
	}
	// Match-level aggregates: actuals from DB for given match
	getBacktestMatchAggregatesActualsFunc = func(_ context.Context, _ int64) (matchAggregates, error) {
		return matchAggregates{}, sql.ErrNoRows
	}
	// Match-level aggregates: predictions from ML given cutoff and teams, with model version
	mlBacktestPredictMatchAggregatesFunc = func(_ context.Context, _ time.Time, _ [2]string) (matchAggregates, string, error) {
		return matchAggregates{}, "", sql.ErrNoRows
	}
	// Historical backtest: delegate to ml-service `/ml/backtest/match`.
	mlHistoricalBacktestFunc = func(ctx context.Context, cutoff time.Time, matchID *int64, filters *HistoricalMatchFilters) (HistoricalBacktestResult, error) {
		client := NewBacktestMLClient()
		return client.historicalMatchBacktest(ctx, cutoff, matchID, filters)
	}
)

// dashboard accuracy-trend seams (overridable in tests)
var listPlayedMatchesByFilters = func(
	ctx context.Context,
	format string,
	team1 string,
	team2 string,
	start time.Time,
	end time.Time,
	order string,
	limit int,
) ([]backtestCandidate, error) {
	// Delegate to DB repository implementation; transform DB rows to server DTO.
	rows, err := db.ListPlayedMatchesByFilters(ctx, format, team1, team2, start, end, order, limit)
	if err != nil {
		return nil, err
	}
	out := make([]backtestCandidate, 0, len(rows))
	for _, r := range rows {
		out = append(out, backtestCandidate{
			MatchID:        r.MatchID,
			StableID:       nullString(r.StableID),
			MatchDate:      r.MatchDate.Format(time.RFC3339),
			Venue:          nullString(r.Venue),
			Season:         nullString(r.Season),
			Format:         nullString(r.FormatCode),
			Team1:          r.Team1,
			Team2:          r.Team2,
			WinnerTeamCode: nullString(r.WinnerTeam),
		})
	}
	return out, nil
}
