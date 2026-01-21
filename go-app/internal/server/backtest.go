package server

import (
	"context"
	"database/sql"
	"errors"
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

// computeAccuracyTrend* and helper utilities moved to backtest_services.go

// Wire default implementations for evaluate-mode seams to DB repos where available.
func init() {
	getBacktestMatchDateFunc = func(ctx context.Context, matchID int64) (time.Time, error) {
		d, err := db.GetMatchDateByID(ctx, matchID)
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
