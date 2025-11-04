package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// WeatherJob represents a row in weather_job queue.
// Sessions is stored as JSONB array (e.g., [{"label":"inning1","at":"..."}]).
type WeatherJob struct {
	ID              int64
	MatchID         int64
	NormalizedVenue string
	City            *string
	Country         *string
	StartAtLocal    *time.Time
	EndAtLocal      *time.Time
	SessionsJSON    []byte
	Status          string
	Attempts        int
	LastError       *string
	ScheduledAt     time.Time
	UpdatedAt       time.Time
}

// EnqueueWeatherJob inserts a job if not exists for the match (idempotent by match_id unique).
func EnqueueWeatherJob(ctx context.Context, j *WeatherJob) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `INSERT INTO weather_job(
		match_id, normalized_venue, city, country, start_at_local, end_at_local, sessions, status
	) VALUES($1,$2,$3,$4,$5,$6,$7,'queued')
	ON CONFLICT (match_id) DO NOTHING`,
		j.MatchID, j.NormalizedVenue, j.City, j.Country, j.StartAtLocal, j.EndAtLocal, j.SessionsJSON,
	)
	return err
}

// DequeueNextWeatherJob fetches one queued job using SKIP LOCKED and marks it working.
func DequeueNextWeatherJob(ctx context.Context, now time.Time) (*WeatherJob, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	tx, err := Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row := tx.QueryRow(
		ctx,
		`SELECT id, match_id, normalized_venue, city, country, start_at_local, end_at_local, sessions, status, attempts, last_error, scheduled_at, updated_at
		FROM weather_job
		WHERE status = 'queued' AND scheduled_at <= $1
		FOR UPDATE SKIP LOCKED LIMIT 1`,
		now,
	)
	var rec WeatherJob
	var city, country pgtype.Text
	var startAt, endAt pgtype.Timestamptz
	var sessions []byte
	var lastErr pgtype.Text
	if err := row.Scan(&rec.ID, &rec.MatchID, &rec.NormalizedVenue, &city, &country, &startAt, &endAt, &sessions, &rec.Status, &rec.Attempts, &lastErr, &rec.ScheduledAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if city.Valid {
		s := city.String
		rec.City = &s
	}
	if country.Valid {
		s := country.String
		rec.Country = &s
	}
	if startAt.Valid {
		t := startAt.Time
		rec.StartAtLocal = &t
	}
	if endAt.Valid {
		t := endAt.Time
		rec.EndAtLocal = &t
	}
	if lastErr.Valid {
		s := lastErr.String
		rec.LastError = &s
	}
	rec.SessionsJSON = sessions
	// mark working
	if _, err := tx.Exec(ctx, `UPDATE weather_job SET status='working', updated_at=$2 WHERE id=$1`, rec.ID, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &rec, nil
}

// MarkWeatherJobDone marks a weather_job row as done by id.
func MarkWeatherJobDone(ctx context.Context, id int64) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(ctx, `UPDATE weather_job SET status='done', updated_at=now() WHERE id=$1`, id)
	return err
}

// MarkWeatherJobFailed re-queues a weather_job with updated attempts, error, and next schedule.
func MarkWeatherJobFailed(ctx context.Context, id int64, attempts int, lastError string, nextSchedule time.Time) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	_, err := Pool.Exec(
		ctx,
		`UPDATE weather_job SET status='queued', attempts=$2, last_error=$3, scheduled_at=$4, updated_at=now() WHERE id=$1`,
		id,
		attempts,
		lastError,
		nextSchedule,
	)
	return err
}
