package db

import "context"

// PoolPlayer is a minimal DTO for team-select pool loading from DB.
type PoolPlayer struct {
	Name      string
	IsBowler  bool
	IsKeeper  bool
	BatScore  float64
	BowlScore float64
}

// TeamSelectRepo abstracts loading a candidate pool from the database.
//
//go:generate mockery --name TeamSelectRepo --output internal/mocks --case underscore
type TeamSelectRepo interface {
	LoadPool(ctx context.Context, matchID int64, format, season string) ([]PoolPlayer, error)
}
