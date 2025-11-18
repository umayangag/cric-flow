package db

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
)

// MatchRepo defines persistence operations for parsed Cricsheet matches.
// NOTE: Keep the interface minimal and focused on importer needs.
//
//go:generate mockery --name MatchRepo --output internal/mocks --case underscore
type MatchRepo interface {
	// UpsertMatches persists a batch of parsed matches atomically if possible.
	UpsertMatches(ctx context.Context, ms []models.Match) error
}
