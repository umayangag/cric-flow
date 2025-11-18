package exportrepo

import (
	"context"

	appdb "github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	exq "github.com/umayangag/cric-info-scrapers/go-app/internal/db/exportqueries"
)

// Repo is a thin adapter that satisfies appdb.DatasetRepo by delegating
// to the read-only helpers in internal/db/exportqueries. It holds no state.
//
// NOTE: This adapter does not open or manage DB connections itself. The
// exportqueries package is responsible for using the application's DB access
// helpers internally when it is implemented. This keeps Repo a simple facade
// that is easy to mock and test.
//
// Behavior is preserved identically to the legacy cmd/export-dataset logic.
// Errors are passed through without logging.
type Repo struct{}

// New constructs a new Repo instance.
func New() *Repo { return &Repo{} }

var _ appdb.DatasetRepo = (*Repo)(nil)

func (r *Repo) BattingUnifiedRows(ctx context.Context) ([][]string, error) {
	return exq.BattingUnifiedRows(ctx)
}

func (r *Repo) BattingLegacyRows(ctx context.Context) ([][]string, error) {
	return exq.BattingLegacyRows(ctx)
}

func (r *Repo) BattingInferenceRows(ctx context.Context, format string) ([][]string, error) {
	return exq.BattingInferenceRows(ctx, format)
}

func (r *Repo) BattingFormatRows(ctx context.Context, format string) ([][]string, error) {
	return exq.BattingFormatRows(ctx, format)
}

func (r *Repo) BowlingUnifiedRows(ctx context.Context) ([][]string, error) {
	return exq.BowlingUnifiedRows(ctx)
}

func (r *Repo) BowlingLegacyRows(ctx context.Context) ([][]string, error) {
	return exq.BowlingLegacyRows(ctx)
}

func (r *Repo) BowlingInferenceRows(ctx context.Context, format string) ([][]string, error) {
	return exq.BowlingInferenceRows(ctx, format)
}

func (r *Repo) BowlingFormatRows(ctx context.Context, format string) ([][]string, error) {
	return exq.BowlingFormatRows(ctx, format)
}
