package db

import (
	"context"
	"errors"
)

// MatchAggregates holds simple match-level actuals used by backtest metrics.
type MatchAggregates struct {
	Runs           float64
	Wickets        float64
	Extras         float64
	WinnerTeamCode string
}

// buildMatchAggregates is a tiny helper to construct MatchAggregates.
// Separated for unit testing of field mapping logic.
func buildMatchAggregates(runs, wickets, extras float64, winner string) MatchAggregates {
	return MatchAggregates{
		Runs:           runs,
		Wickets:        wickets,
		Extras:         extras,
		WinnerTeamCode: winner,
	}
}

// GetMatchAggregates returns basic aggregates for a match. Numeric totals are
// summed from match_inning (runs_scored, wickets_lost, extras), and
// winner is derived from team_match.
func GetMatchAggregates(ctx context.Context, matchID int64) (MatchAggregates, error) {
	if Pool == nil {
		return MatchAggregates{}, errors.New("db pool not initialized")
	}

	// 1) Sum numeric totals from match_inning across all innings.
	var (
		runs    float64
		wickets float64
		extras  float64
	)
	if err := Pool.QueryRow(ctx, `
        SELECT
            COALESCE(SUM(mi.runs_scored), 0),
            COALESCE(SUM(mi.wickets_lost), 0),
            COALESCE(SUM(mi.extras), 0)
        FROM match_inning mi
        WHERE mi.match_id = $1
    `, matchID).Scan(&runs, &wickets, &extras); err != nil {
		// If schema differs or row missing, keep zeros for totals
		runs, wickets, extras = 0, 0, 0
	}

	// 2) Determine winner team name/code from team_match results mapping.
	//    If schema differs, adjust mapping accordingly in future iterations.
	var winner string
	if err := Pool.QueryRow(ctx, `
        SELECT COALESCE(t.name, '') AS winner
        FROM team_match tm
        JOIN team t ON t.id = tm.team_id
        WHERE tm.match_id = $1 AND UPPER(COALESCE(tm.result,'') ) IN ('W','WIN','1','TRUE','T')
        LIMIT 1
    `, matchID).Scan(&winner); err != nil {
		// It's okay if not found; we return empty winner
		winner = ""
	}

	return buildMatchAggregates(runs, wickets, extras, winner), nil
}

// GetAverageExtrasForFormat returns the average total extras per match (sum of extras
// across all innings per match) for matches with the given format and optional venue.
// Used to predict extras when no per-player extras model exists. When venueID is nil or 0, only format is used.
// Returns 0 when no matching rows (e.g. no data for that format).
func GetAverageExtrasForFormat(ctx context.Context, formatID int64, venueID *int64) (float64, error) {
	if Pool == nil {
		return 0, errors.New("db pool not initialized")
	}
	var avg float64
	query := `
		SELECT COALESCE(AVG(tot.extras), 0)
		FROM (
			SELECT mi.match_id, SUM(mi.extras) AS extras
			FROM match_inning mi
			JOIN match m ON m.match_id = mi.match_id
			WHERE m.format_id = $1
			  AND ($2::bigint IS NULL OR $2 = 0 OR m.venue_id = $2)
			GROUP BY mi.match_id
		) tot
	`
	err := Pool.QueryRow(ctx, query, formatID, venueID).Scan(&avg)
	if err != nil {
		return 0, err
	}
	return avg, nil
}
