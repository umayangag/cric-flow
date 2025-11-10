package db

import "context"

// BackfillEvent represents a simplified fielding action used by backfill service.
// Named uniquely to avoid clashing with existing FieldingEvent types in this module.
type BackfillEvent struct {
	MatchID           int64
	PlayerID          int64
	Catches           int
	RunOuts           int
	Stumpings         int
	RunoutsDirectHits int
}

// FieldingAggregateRow is the per-player, per-match accumulated result written
// into the destination table used by exporters.
type FieldingAggregateRow struct {
	MatchID           int64
	PlayerID          int64
	Catches           int
	RunOuts           int
	Stumpings         int
	RunoutsDirectHits int
}

// FieldingRepo provides the minimal persistence API for backfilling fielding aggregates.
//go:generate mockery --name FieldingRepo --output internal/mocks --case underscore
// NOTE: Interfaces live close to consumers and are intentionally tiny.
type FieldingRepo interface {
	// ListFieldingEvents returns fielding events filtered by matchID when provided.
	// When matchID is nil, returns events for all matches.
	ListFieldingEvents(ctx context.Context, matchID *int64) ([]BackfillEvent, error)
	// UpsertFieldingAggregates persists the computed aggregates.
	UpsertFieldingAggregates(ctx context.Context, rows []FieldingAggregateRow) error
}
