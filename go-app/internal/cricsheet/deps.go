package cricsheet

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	weatherSvc "github.com/umayangag/cric-info-scrapers/go-app/internal/weather/service"
)

// CricsheetDB abstracts DB operations used by ingest for testability.
// nolint:revive // name stutter is intentional to match package domain terms
type CricsheetDB interface {
	GetMatchFormatIDByCode(ctx context.Context, code string) (int64, error)
	EnsureMatchWithFormat(ctx context.Context, matchID int64, formatID int64) error
	GetOrCreateVenue(ctx context.Context, name string) (int64, error)
	GetOrCreateSeason(ctx context.Context, name string) (int64, error)
	GetOrCreateOpposition(ctx context.Context, name string) (int64, error)
	UpdateMatchDetails(ctx context.Context, matchID int64, upd *db.MatchInfoUpdate) error
	GetOrCreateByName(ctx context.Context, name string) (int64, error)
	UpsertBatting(ctx context.Context, b *db.Batting) error
	UpsertBowling(ctx context.Context, b *db.Bowling) error
	UpsertFielding(ctx context.Context, f *db.Fielding) error
	Exec(ctx context.Context, sql string, args ...any) error
}

// WeatherClient abstracts weather job enqueueing for testability.
type WeatherClient interface {
	EnqueueJob(ctx context.Context, matchID int64, city, venue string, innings int) error
}

// Default adapters
var (
	cricDB             CricsheetDB   = realDB{}
	weatherClient      WeatherClient = realWeather{}
	recomputeFn                      = db.RecomputeFieldingAggregates
	insertBallEventsFn               = db.InsertBallEvents
)

// SetCricsheetDB allows tests to inject a fake DB implementation.
func SetCricsheetDB(d CricsheetDB) { cricDB = d }

// SetWeatherClient allows tests to inject a fake weather client.
func SetWeatherClient(w WeatherClient) { weatherClient = w }

// SetRecomputeFn allows tests to stub out the recompute function.
func SetRecomputeFn(f func(ctx context.Context, matchID int64) error) { recomputeFn = f }

type realDB struct{}

type realWeather struct{}

func (realDB) GetMatchFormatIDByCode(ctx context.Context, code string) (int64, error) {
	return db.GetMatchFormatIDByCode(ctx, code)
}

func (realDB) EnsureMatchWithFormat(ctx context.Context, matchID int64, formatID int64) error {
	return db.EnsureMatchWithFormat(ctx, matchID, formatID)
}

func (realDB) GetOrCreateVenue(ctx context.Context, name string) (int64, error) {
	return db.GetOrCreateVenue(ctx, name)
}

func (realDB) GetOrCreateSeason(ctx context.Context, name string) (int64, error) {
	return db.GetOrCreateSeason(ctx, name)
}

func (realDB) GetOrCreateOpposition(ctx context.Context, name string) (int64, error) {
	return db.GetOrCreateOpposition(ctx, name)
}

func (realDB) UpdateMatchDetails(ctx context.Context, matchID int64, upd *db.MatchInfoUpdate) error {
	return db.UpdateMatchDetails(ctx, matchID, upd)
}

func (realDB) GetOrCreateByName(ctx context.Context, name string) (int64, error) {
	return db.GetOrCreateByName(ctx, name)
}

func (realDB) UpsertBatting(ctx context.Context, b *db.Batting) error {
	return db.UpsertBatting(ctx, b)
}

func (realDB) UpsertBowling(ctx context.Context, b *db.Bowling) error {
	return db.UpsertBowling(ctx, b)
}

func (realDB) UpsertFielding(ctx context.Context, f *db.Fielding) error {
	return db.UpsertFielding(ctx, f)
}

func (realDB) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := db.Pool.Exec(ctx, sql, args...)
	return err
}

func (realWeather) EnqueueJob(ctx context.Context, matchID int64, city, venue string, innings int) error {
	return weatherSvc.EnqueueJob(ctx, matchID, city, venue, innings)
}
