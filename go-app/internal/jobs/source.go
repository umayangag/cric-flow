package jobs

import "context"

// Source is a minimal job source that yields match IDs to process in batches.
//
//go:generate mockery --name Source --output internal/mocks --case underscore
type Source interface {
	// Next returns up to `batch` match IDs to process.
	// ok=false indicates the source is exhausted.
	Next(ctx context.Context, batch int) (ids []int64, ok bool, err error)
}
