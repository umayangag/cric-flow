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

// predictionColumns is the row a read returns, in one place so the three readers cannot
// select different things and scan the same way.
//
// Both opposition ids come back folded onto their club, COALESCE(canonical_id, id). The
// insert writes the club id ResolveTeamSide gave the answer, but a club can be renamed
// after a prediction is filed -- the importer writes opposition.canonical_id at the end of
// a run, from configs/team_lineage.json -- and from that moment the stored id is a
// superseded row. Folding on read means the id a reader gets is the club, whenever the
// rename happened, which is what lets the track record compare it against a match's sides
// without either space going stale under the other (GO-02).
const predictionColumns = `p.id, p.issued_at, p.run_id, p.ratings_through, p.format_code,
	COALESCE(team1.canonical_id, team1.id), COALESCE(team2.canonical_id, team2.id),
	p.gender, p.match_date, p.selection_objective,
	p.win_probability_team1, p.win_probability_source, p.simulator_shared_factor`

// predictionFrom carries the two joins predictionColumns folds through. Both are inner
// joins on NOT NULL foreign keys to opposition (migration 0013), so no row is lost.
const predictionFrom = `
	FROM issued_prediction p
	JOIN opposition team1 ON team1.id = p.team1_opposition_id
	JOIN opposition team2 ON team2.id = p.team2_opposition_id`

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
		   simulator_shared_factor, request, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`,
		prediction.ID, prediction.IssuedAt, prediction.RunID, prediction.RatingsThrough,
		prediction.FormatCode, prediction.Team1OppositionID, prediction.Team2OppositionID,
		prediction.Gender, prediction.MatchDate, prediction.SelectionObjective,
		prediction.WinProbabilityTeam1, prediction.WinProbabilitySource,
		prediction.SimulatorSharedFactor,
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
		`SELECT `+predictionColumns+`, p.request, p.payload`+predictionFrom+`
		 WHERE p.id = $1`, id).
		Scan(&stored.ID, &stored.IssuedAt, &stored.RunID, &stored.RatingsThrough,
			&stored.FormatCode, &stored.Team1OppositionID, &stored.Team2OppositionID,
			&stored.Gender, &stored.MatchDate, &stored.SelectionObjective,
			&stored.WinProbabilityTeam1, &stored.WinProbabilitySource,
			&stored.SimulatorSharedFactor, &request, &payload)
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
		`SELECT `+predictionColumns+predictionFrom+`
		 ORDER BY p.issued_at DESC, p.id DESC LIMIT $1 OFFSET $2`,
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
			&stored.WinProbabilityTeam1, &stored.WinProbabilitySource,
			&stored.SimulatorSharedFactor); err != nil {
			return predictions.Page{}, fmt.Errorf("list predictions: %w", err)
		}
		page.Predictions = append(page.Predictions, stored)
	}
	if err := rows.Err(); err != nil {
		return predictions.Page{}, fmt.Errorf("list predictions: %w", err)
	}
	return page, nil
}

// All returns the whole record, oldest first, payloads included (P2-4).
//
// Unpaged on purpose: the track record decides which forecast of a fixture is the last
// one issued, which is a question about every row, and it reads the ranges and the
// elevens out of each payload. One operator's record is tens of rows; at ten kilobytes a
// payload that is a single small read, and a page would only make the answer wrong.
func (s *PredictionStore) All(ctx context.Context) ([]predictions.Prediction, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx,
		`SELECT `+predictionColumns+`, p.request, p.payload`+predictionFrom+`
		 ORDER BY p.issued_at ASC, p.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("read the prediction record: %w", err)
	}
	defer rows.Close()

	record := make([]predictions.Prediction, 0, 64)
	for rows.Next() {
		var stored predictions.Prediction
		var request, payload []byte
		if err := rows.Scan(&stored.ID, &stored.IssuedAt, &stored.RunID, &stored.RatingsThrough,
			&stored.FormatCode, &stored.Team1OppositionID, &stored.Team2OppositionID,
			&stored.Gender, &stored.MatchDate, &stored.SelectionObjective,
			&stored.WinProbabilityTeam1, &stored.WinProbabilitySource,
			&stored.SimulatorSharedFactor, &request, &payload); err != nil {
			return nil, fmt.Errorf("read the prediction record: %w", err)
		}
		stored.Request = request
		stored.Payload = payload
		record = append(record, stored)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the prediction record: %w", err)
	}
	return record, nil
}
