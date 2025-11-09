package exportqueries

import (
	"context"
	"errors"
)

// BattingUnifiedRows should return CSV-shaped rows for the unified batting export.
// TODO: Extract the existing SQL from cmd/export-dataset and implement this.
func BattingUnifiedRows(_ context.Context) ([][]string, error) {
	return nil, errors.New("exportqueries: BattingUnifiedRows not implemented yet")
}

// BattingLegacyRows should return CSV-shaped rows for the legacy (combined) batting export.
// TODO: Extract the existing SQL from cmd/export-dataset and implement this.
func BattingLegacyRows(_ context.Context) ([][]string, error) {
	return nil, errors.New("exportqueries: BattingLegacyRows not implemented yet")
}

// BattingInferenceRows should return CSV-shaped rows for the batting inference export filtered by format.
// TODO: Extract the existing SQL from cmd/export-dataset and implement this.
func BattingInferenceRows(_ context.Context, _ string) ([][]string, error) {
	return nil, errors.New("exportqueries: BattingInferenceRows not implemented yet")
}
