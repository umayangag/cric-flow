package db

import "context"

// DatasetRepo defines minimal data access for CSV exporters.
// NOTE: This is intentionally small; we will evolve method sets as more logic is
// extracted from cmd/export-dataset. Methods return rows already shaped for CSV
// (including header row as the first entry if desired by the service layer).
//
//go:generate mockery --name DatasetRepo --output internal/mocks --case underscore
type DatasetRepo interface {
	// BattingUnifiedRows returns rows for the unified batting export.
	BattingUnifiedRows(ctx context.Context) ([][]string, error)
	// BattingLegacyRows returns rows for the legacy batting export (no format filter).
	BattingLegacyRows(ctx context.Context) ([][]string, error)
	// BattingInferenceRows returns rows for batting inference export filtered by format.
	BattingInferenceRows(ctx context.Context, format string) ([][]string, error)
	// BattingFormatRows returns rows for per-format training batting export.
	BattingFormatRows(ctx context.Context, format string) ([][]string, error)

	// BowlingUnifiedRows returns rows for the unified bowling export.
	BowlingUnifiedRows(ctx context.Context) ([][]string, error)
	// BowlingLegacyRows returns rows for the legacy bowling export (no format filter).
	BowlingLegacyRows(ctx context.Context) ([][]string, error)
	// BowlingInferenceRows returns rows for bowling inference export filtered by format.
	BowlingInferenceRows(ctx context.Context, format string) ([][]string, error)
	// BowlingFormatRows returns rows for per-format training bowling export.
	BowlingFormatRows(ctx context.Context, format string) ([][]string, error)
}
