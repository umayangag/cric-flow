package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildMatchAggregates_TableDriven ensures the helper constructs the
// struct with expected field assignments for various inputs (mapping semantics
// used by GetMatchAggregates: score -> Runs, wickets -> Wickets, extras -> Extras).
func TestBuildMatchAggregates_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		runs    float64
		wickets float64
		extras  float64
		winner  string
	}{
		{"simple mapping", 150, 7, 10, "IND"},
		{"zeros", 0, 0, 0, ""},
		{"empty winner", 200, 10, 5, ""},
		{"fractional runs", 99.5, 3, 2, "AUS"},
		{"large values", 500, 20, 25, "PAK"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildMatchAggregates(tt.runs, tt.wickets, tt.extras, tt.winner)
			require.Equal(t, MatchAggregates{
				Runs:           tt.runs,
				Wickets:        tt.wickets,
				Extras:         tt.extras,
				WinnerTeamCode: tt.winner,
			}, got)
		})
	}
}

// TestGetAverageExtrasForFormat_PoolNil ensures we return an error when the db pool is not initialized.
func TestGetAverageExtrasForFormat_PoolNil(t *testing.T) {
	orig := Pool
	defer func() { Pool = orig }()
	Pool = nil

	avg, err := GetAverageExtrasForFormat(context.TODO(), 1, nil)
	if err == nil {
		t.Fatalf("expected error when pool is nil, got avg=%v", avg)
	}
	if avg != 0 {
		t.Fatalf("expected 0 on error, got %v", avg)
	}
}
