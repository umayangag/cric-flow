package exportqueries

import (
	"context"
	"time"
)

// Repo implements db.DatasetRepo using local functions.
type Repo struct{}

func (r *Repo) BattingUnifiedRows(ctx context.Context) ([][]string, error) {
	return BattingUnifiedRows(ctx)
}

func (r *Repo) BattingLegacyRows(ctx context.Context) ([][]string, error) {
	return BattingLegacyRows(ctx)
}

func (r *Repo) BattingInferenceRows(ctx context.Context, format string) ([][]string, error) {
	return BattingInferenceRows(ctx, format)
}

func (r *Repo) BattingFormatRows(ctx context.Context, format string) ([][]string, error) {
	return BattingFormatRows(ctx, format)
}

func (r *Repo) BowlingUnifiedRows(ctx context.Context) ([][]string, error) {
	return BowlingUnifiedRows(ctx)
}

func (r *Repo) BowlingLegacyRows(ctx context.Context) ([][]string, error) {
	return BowlingLegacyRows(ctx)
}

func (r *Repo) BowlingInferenceRows(ctx context.Context, format string) ([][]string, error) {
	return BowlingInferenceRows(ctx, format)
}

func (r *Repo) BowlingFormatRows(ctx context.Context, format string) ([][]string, error) {
	return BowlingFormatRows(ctx, format)
}

func (r *Repo) FieldingUnifiedRows(ctx context.Context) ([][]string, error) {
	return FieldingTrainingRows(ctx, time.Now())
}

func (r *Repo) FieldingFormatRows(ctx context.Context, format string) ([][]string, error) {
	return FieldingTrainingRowsWithFormat(ctx, format, time.Now())
}
