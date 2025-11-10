package db

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/domain"
)

// MatchRepo defines persistence operations for parsed Cricsheet matches.
//go:generate mockery --name MatchRepo --output internal/mocks --case underscore
// NOTE: Keep the interface minimal and focused on importer needs.
type MatchRepo interface {
	// UpsertMatches persists a batch of parsed matches atomically if possible.
	UpsertMatches(ctx context.Context, ms []domain.Match) error
}
