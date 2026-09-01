package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/config"
)

// MatchAfterItem holds match_id and match_date for walk-forward (matches after a cutoff).
type MatchAfterItem struct {
	MatchID   int64
	MatchDate time.Time
}

// ListMatchIDsAfter returns match_id and match_date for matches strictly after the given cutoff,
// for the given format IDs (e.g. from GetFormatIDsForTrainingBucket). Order: match_date ASC.
func ListMatchIDsAfter(ctx context.Context, formatIDs []int64, after time.Time, limit int) ([]MatchAfterItem, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	if limit <= 0 {
		limit = config.BacktestListDefaultLimit(config.Load())
	}
	q := `SELECT m.match_id, m.match_date
		FROM match m
		WHERE m.format_id = ANY($1::bigint[]) AND m.match_date > $2
		ORDER BY m.match_date ASC
		LIMIT $3`
	rows, err := Pool.Query(ctx, q, formatIDs, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MatchAfterItem
	for rows.Next() {
		var item MatchAfterItem
		if err := rows.Scan(&item.MatchID, &item.MatchDate); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// MatchPredictionAggregates stores cached match-level prediction outputs.
type MatchPredictionAggregates struct {
	MatchID             int64
	Format              string
	Team1Code           string
	Team2Code           string
	PredictedWinnerCode sql.NullString
	PredictedTotalRuns  sql.NullFloat64
	ModelVersion        sql.NullString
	CutoffAt            time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
