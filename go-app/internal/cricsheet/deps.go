package cricsheet

import (
	"context"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// CricsheetDB abstracts DB operations used by ingest for testability.
// nolint:revive // name stutter is intentional to match package domain terms
type CricsheetDB interface {
	GetMatchFormatIDByCode(ctx context.Context, code string) (int64, error)
	UpsertMatch(ctx context.Context, m *db.MatchInsert) error
	UpsertMatchInning(ctx context.Context, mi *db.MatchInningInsert) error
	GetOrCreateSeason(ctx context.Context, name string) (int64, error)
	GetOrCreateOpposition(ctx context.Context, name, gender string) (int64, error)
	GetOrCreatePlayer(ctx context.Context, externalID, name, nameAsOf string) (int64, string, error)
	UpdatePlayerDisplayNames(ctx context.Context, names []db.PlayerDisplayName) error
	ApplyTeamLineage(ctx context.Context, renames []db.TeamRename) (db.TeamLineageReport, error)
	UpsertBatting(ctx context.Context, b *db.Batting) error
	UpsertBattingBatch(ctx context.Context, rows []db.Batting) error
	UpsertBowling(ctx context.Context, b *db.Bowling) error
	UpsertBowlingBatch(ctx context.Context, rows []db.Bowling) error
	UpsertFielding(ctx context.Context, f *db.Fielding) error
	UpsertFieldingBatch(ctx context.Context, rows []db.Fielding) error
	Exec(ctx context.Context, sql string, args ...any) error
}

// Default adapters
var (
	cricDB CricsheetDB = realDB{}
)

// SetCricsheetDB allows tests to inject a fake DB implementation.
func SetCricsheetDB(d CricsheetDB) { cricDB = d }

// GetCricsheetDB returns the current DB implementation (for tests).
func GetCricsheetDB() CricsheetDB { return cricDB }

// RunInTxFn, when set, replaces db.RunInTx for transaction execution. Used by tests to inject
// failing or spy transactions without mocking the full pool.
var runInTxFn func(ctx context.Context, fn func(ctx context.Context, tx db.CopyFromTx) error) error

// SetRunInTxFn allows tests to inject custom transaction behavior.
func SetRunInTxFn(fn func(ctx context.Context, inner func(ctx context.Context, tx db.CopyFromTx) error) error) {
	runInTxFn = fn
}

type realDB struct{}

func (realDB) GetMatchFormatIDByCode(ctx context.Context, code string) (int64, error) {
	return db.GetMatchFormatIDByCode(ctx, code)
}

func (realDB) UpsertMatch(ctx context.Context, m *db.MatchInsert) error {
	return db.UpsertMatch(ctx, m)
}

func (realDB) UpsertMatchInning(ctx context.Context, mi *db.MatchInningInsert) error {
	return db.UpsertMatchInning(ctx, mi)
}

func (realDB) GetOrCreateSeason(ctx context.Context, name string) (int64, error) {
	return db.GetOrCreateSeason(ctx, name)
}

func (realDB) GetOrCreateOpposition(ctx context.Context, name, gender string) (int64, error) {
	return db.GetOrCreateOpposition(ctx, name, gender)
}

func (realDB) GetOrCreatePlayer(ctx context.Context, externalID, name, nameAsOf string) (int64, string, error) {
	return db.GetOrCreatePlayer(ctx, externalID, name, nameAsOf)
}

func (realDB) UpdatePlayerDisplayNames(ctx context.Context, names []db.PlayerDisplayName) error {
	return db.UpdatePlayerDisplayNames(ctx, names)
}

func (realDB) ApplyTeamLineage(ctx context.Context, renames []db.TeamRename) (db.TeamLineageReport, error) {
	return db.ApplyTeamLineage(ctx, renames)
}

func (realDB) UpsertBatting(ctx context.Context, b *db.Batting) error {
	return db.UpsertBatting(ctx, b)
}

func (realDB) UpsertBattingBatch(ctx context.Context, rows []db.Batting) error {
	return db.UpsertBattingBatch(ctx, rows)
}

func (realDB) UpsertBowling(ctx context.Context, b *db.Bowling) error {
	return db.UpsertBowling(ctx, b)
}

func (realDB) UpsertBowlingBatch(ctx context.Context, rows []db.Bowling) error {
	return db.UpsertBowlingBatch(ctx, rows)
}

func (realDB) UpsertFielding(ctx context.Context, f *db.Fielding) error {
	return db.UpsertFielding(ctx, f)
}

func (realDB) UpsertFieldingBatch(ctx context.Context, rows []db.Fielding) error {
	return db.UpsertFieldingBatch(ctx, rows)
}

func (realDB) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := db.Pool.Exec(ctx, sql, args...)
	return err
}
