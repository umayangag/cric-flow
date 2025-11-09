package exportqueries

import (
	"context"
	"errors"
)

// BowlingUnifiedRows should return CSV-shaped rows for the unified bowling export.
// TODO: Extract the existing SQL from cmd/export-dataset and implement this.
func BowlingUnifiedRows(_ context.Context) ([][]string, error) {
	return nil, errors.New("exportqueries: BowlingUnifiedRows not implemented yet")
}

// BowlingLegacyRows should return CSV-shaped rows for the legacy (combined) bowling export.
// TODO: Extract the existing SQL from cmd/export-dataset and implement this.
func BowlingLegacyRows(_ context.Context) ([][]string, error) {
	return nil, errors.New("exportqueries: BowlingLegacyRows not implemented yet")
}

// BowlingInferenceRows should return CSV-shaped rows for the bowling inference export filtered by format.
// TODO: Extract the existing SQL from cmd/export-dataset and implement this.
func BowlingInferenceRows(_ context.Context, _ string) ([][]string, error) {
	return nil, errors.New("exportqueries: BowlingInferenceRows not implemented yet")
}
