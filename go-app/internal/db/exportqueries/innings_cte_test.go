package exportqueries

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInningsRunsHoldoutCTE_ReturnsExpectedFragment(t *testing.T) {
	got := inningsRunsHoldoutCTE("innings_sums")
	require.Contains(t, got, "innings_sums AS (")
	require.Contains(t, got, "SELECT match_id, inning_number, SUM(runs)::bigint AS total_runs")
	require.Contains(t, got, "FROM batting_data")
	require.Contains(t, got, "WHERE match_id = ANY($1::bigint[])")
	require.Contains(t, got, "GROUP BY match_id, inning_number")
}

func TestBattingHoldoutRawQuery_UsesSharedInningsRunsCTE(t *testing.T) {
	q, args := battingHoldoutRawQuery([]int64{100, 200})
	require.Len(t, args, 1)
	// Shared CTE semantics: runs from batting_data, grouped by match_id/inning_number, filtered by match IDs
	require.Contains(t, q, "innings_sums AS (")
	require.Contains(t, q, "FROM batting_data")
	require.Contains(t, q, "SUM(runs)::bigint AS total_runs")
	require.Contains(t, q, "match_id = ANY($1::bigint[])")
	require.Contains(
		t,
		q,
		"LEFT JOIN innings_sums ins ON ins.match_id = bd.match_id AND ins.inning_number = bd.inning_number",
	)
}

func TestBowlingHoldoutRawQuery_UsesSharedInningsRunsCTE(t *testing.T) {
	q, args := bowlingHoldoutRawQuery([]int64{100})
	require.Len(t, args, 1)
	// Bowling holdout uses same innings-runs CTE (from batting_data) for innings_runs column
	require.Contains(t, q, "innings_runs_cte AS (")
	require.Contains(t, q, "FROM batting_data")
	require.True(t, strings.Contains(q, "SUM(runs)::bigint AS total_runs"), "query should use shared runs aggregation")
	require.Contains(t, q, "match_id = ANY($1::bigint[])")
	require.Contains(
		t,
		q,
		"LEFT JOIN innings_runs_cte ir ON ir.match_id = b.match_id AND ir.inning_number = b.inning_number",
	)
}
