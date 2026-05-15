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
	t.Parallel()
	testCases := []struct {
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
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildMatchAggregates(tc.runs, tc.wickets, tc.extras, tc.winner)
			require.Equal(t, MatchAggregates{
				Runs:           tc.runs,
				Wickets:        tc.wickets,
				Extras:         tc.extras,
				WinnerTeamCode: tc.winner,
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
	require.Error(t, err, "expected error when pool is nil")
	require.Equal(t, float64(0), avg, "expected 0 on error")
}
