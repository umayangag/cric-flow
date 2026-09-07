package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/umayangag/cric-flow/go-app/internal/predictions"
)

// PredictionStore is the prediction record's persistence (migration 0013). It implements
// predictions.Store; there are no rules to apply here, so this file holds SQL and nothing
// else.
type PredictionStore struct{}

// NewPredictionStore returns the record backed by the process's connection pool.
func NewPredictionStore() *PredictionStore { return &PredictionStore{} }

// predictionColumns is the row a read returns, in one place so the two readers cannot
// select different things and scan the same way.
const predictionColumns = `id, issued_at, run_id, ratings_through, format_code,
	team1_opposition_id, team2_opposition_id, gender, match_date, selection_objective,
	win_probability_team1, win_probability_source`

// Record files one issued answer.
//
// A single INSERT: an answer is one row, and there is no history to append beside it the
// way the retirement ledger has one — a prediction is never edited, so the row *is* the
// history. A duplicate id would be the same answer filed twice and is refused by the
// primary key rather than merged, which is why there is no ON CONFLICT clause here.
func (s *PredictionStore) Record(ctx context.Context, prediction predictions.Prediction) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `
		INSERT INTO issued_prediction
		  (id, issued_at, run_id, ratings_through, format_code,
		   team1_opposition_id, team2_opposition_id, gender, match_date,
		   selection_objective, win_probability_team1, win_probability_source,
		   request, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`,
		prediction.ID, prediction.IssuedAt, prediction.RunID, prediction.RatingsThrough,
		prediction.FormatCode, prediction.Team1OppositionID, prediction.Team2OppositionID,
		prediction.Gender, prediction.MatchDate, prediction.SelectionObjective,
		prediction.WinProbabilityTeam1, prediction.WinProbabilitySource,
		[]byte(prediction.Request), []byte(prediction.Payload))
	if err != nil {
		return fmt.Errorf("record prediction: %w", err)
	}
	return nil
}

// Get returns one stored answer whole, both documents included.
func (s *PredictionStore) Get(ctx context.Context, id string) (*predictions.Prediction, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	var stored predictions.Prediction
	var request, payload []byte
	err := Pool.QueryRow(ctx,
		`SELECT `+predictionColumns+`, request, payload
		 FROM issued_prediction WHERE id = $1`, id).
		Scan(&stored.ID, &stored.IssuedAt, &stored.RunID, &stored.RatingsThrough,
			&stored.FormatCode, &stored.Team1OppositionID, &stored.Team2OppositionID,
			&stored.Gender, &stored.MatchDate, &stored.SelectionObjective,
			&stored.WinProbabilityTeam1, &stored.WinProbabilitySource, &request, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, predictions.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read prediction %s: %w", id, err)
	}
	stored.Request = request
	stored.Payload = payload
	return &stored, nil
}

// List returns one page of the record, newest first, with the count it was taken out of.
//
// The payload is deliberately not selected: a listing is the record's index, and 22
// players' forecasts per row would make it a multi-megabyte answer for a page of twenty.
// A caller who wants the claim itself asks for the row by id.
func (s *PredictionStore) List(
	ctx context.Context,
	query predictions.Query,
) (predictions.Page, error) {
	if Pool == nil {
		return predictions.Page{}, errors.New("db pool not initialized")
	}
	var page predictions.Page
	if err := Pool.QueryRow(ctx, `SELECT count(*) FROM issued_prediction`).Scan(&page.Total); err != nil {
		return predictions.Page{}, fmt.Errorf("count predictions: %w", err)
	}
	rows, err := Pool.Query(ctx,
		`SELECT `+predictionColumns+`
		 FROM issued_prediction ORDER BY issued_at DESC, id DESC LIMIT $1 OFFSET $2`,
		query.Limit, query.Offset)
	if err != nil {
		return predictions.Page{}, fmt.Errorf("list predictions: %w", err)
	}
	defer rows.Close()

	page.Predictions = make([]predictions.Prediction, 0, query.Limit)
	for rows.Next() {
		var stored predictions.Prediction
		if err := rows.Scan(&stored.ID, &stored.IssuedAt, &stored.RunID, &stored.RatingsThrough,
			&stored.FormatCode, &stored.Team1OppositionID, &stored.Team2OppositionID,
			&stored.Gender, &stored.MatchDate, &stored.SelectionObjective,
			&stored.WinProbabilityTeam1, &stored.WinProbabilitySource); err != nil {
			return predictions.Page{}, fmt.Errorf("list predictions: %w", err)
		}
		page.Predictions = append(page.Predictions, stored)
	}
	if err := rows.Err(); err != nil {
		return predictions.Page{}, fmt.Errorf("list predictions: %w", err)
	}
	return page, nil
}
