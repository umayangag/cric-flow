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

// GetMatchPredictionAggregates fetches a cached aggregates record for the match.
func GetMatchPredictionAggregates(ctx context.Context, matchID int64) (MatchPredictionAggregates, error) {
	if Pool == nil {
		return MatchPredictionAggregates{}, errors.New("db pool not initialized")
	}
	const q = `
        SELECT match_id, format, team1_code, team2_code,
               predicted_winner_code, predicted_total_runs, model_version,
               cutoff_at, created_at, updated_at
        FROM match_prediction_aggregates
        WHERE match_id = $1`
	var row MatchPredictionAggregates
	err := Pool.QueryRow(ctx, q, matchID).Scan(
		&row.MatchID,
		&row.Format,
		&row.Team1Code,
		&row.Team2Code,
		&row.PredictedWinnerCode,
		&row.PredictedTotalRuns,
		&row.ModelVersion,
		&row.CutoffAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return MatchPredictionAggregates{}, err
	}
	return row, nil
}

// UpsertMatchPredictionAggregates inserts or updates a cached aggregates record for the match.
func UpsertMatchPredictionAggregates(ctx context.Context, row MatchPredictionAggregates) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	const q = `
        INSERT INTO match_prediction_aggregates (
            match_id, format, team1_code, team2_code,
            predicted_winner_code, predicted_total_runs, model_version,
            cutoff_at
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
        ON CONFLICT (match_id) DO UPDATE SET
            format = EXCLUDED.format,
            team1_code = EXCLUDED.team1_code,
            team2_code = EXCLUDED.team2_code,
            predicted_winner_code = EXCLUDED.predicted_winner_code,
            predicted_total_runs = EXCLUDED.predicted_total_runs,
            model_version = EXCLUDED.model_version,
            cutoff_at = EXCLUDED.cutoff_at`
	_, err := Pool.Exec(
		ctx, q,
		row.MatchID,
		row.Format,
		row.Team1Code,
		row.Team2Code,
		row.PredictedWinnerCode,
		row.PredictedTotalRuns,
		row.ModelVersion,
		row.CutoffAt,
	)
	return err
}
