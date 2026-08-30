package exportqueries

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWinQueryStructure ensures the built win training query has valid WITH/SELECT structure:
// no trailing comma after the last CTE (would cause "syntax error at or near SELECT" in PostgreSQL).
func TestWinQueryStructure(t *testing.T) {
	ctes := buildWinAggAndTop3CTEs()
	require.NotEmpty(t, ctes, "buildWinAggAndTop3CTEs should return non-empty string")
	require.False(t, strings.HasSuffix(ctes, ","), "last CTE must not end with comma (would break WITH .. SELECT)")
	require.True(t, strings.HasSuffix(ctes, ")"), "last CTE should end with closing paren")

	cols := buildWinFeatureSelectColumns()
	require.NotEmpty(t, cols)
	// SELECT list: comma between groups, no trailing comma before FROM
	require.False(
		t,
		strings.HasSuffix(strings.TrimSpace(cols), ","),
		"last select column group must not end with comma",
	)
}

// TestBuildWinAggAndTop3CTEs_ContainsAllFeatureCTEs asserts the built CTE string includes
// agg_* and top3_* CTEs for each of the 8 win feature names (t1_bat_cons, t1_bowl_cons, ...).
func TestBuildWinAggAndTop3CTEs_ContainsAllFeatureCTEs(t *testing.T) {
	ctes := buildWinAggAndTop3CTEs()
	for _, name := range winFeatureCTENames() {
		name := name // capture range variable
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, ctes, "agg_"+name, "CTEs must include agg_%s", name)
			require.Contains(t, ctes, "top3_"+name, "CTEs must include top3_%s", name)
		})
	}
	// Sanity: top3 pattern (ROW_NUMBER, rn <= 3, top3_mean)
	require.Contains(t, ctes, "ROW_NUMBER()")
	require.Contains(t, ctes, "rn <= 3")
	require.Contains(t, ctes, "top3_mean")
}

// TestBuildWinFeatureSelectColumns_ContainsAllFeatureColumns asserts the built SELECT columns
// include all 8 feature aliases (a1..a8) and top3 aliases (t3a1..t3a8) with the six stats.
func TestBuildWinFeatureSelectColumns_ContainsAllFeatureColumns(t *testing.T) {
	cols := buildWinFeatureSelectColumns()
	for n := 1; n <= 8; n++ {
		require.Contains(t, cols, fmt.Sprintf("a%d.", n), "columns must reference agg alias a%d", n)
		require.Contains(t, cols, fmt.Sprintf("t3a%d.", n), "columns must reference top3 alias t3a%d", n)
	}
	require.Contains(t, cols, "top3_mean", "columns must include top3_mean")
	// The count is gone on purpose (S-3c): over a squad it is the squad size, constant
	// across candidate XIs at selection time, and it was how the result leaked in.
	require.NotContains(t, cols, ".cnt", "the count stat must not come back")
}

// TestWinExport_AggregatesOverTheSquadNotTheScorecard is the S-3c guard. batting_data and
// bowling_data membership is decided by how the match went, so reading them made the
// number of team-2 batters predict the result at held-out AUC 0.89-0.94.
func TestWinExport_AggregatesOverTheSquadNotTheScorecard(t *testing.T) {
	q := winEnhancedQueryForTest()

	require.Contains(t, q, "FROM match_player mp", "squads must come from match_player")
	require.NotContains(t, q, "FROM batting_data", "the scorecard must not define the population")
	require.NotContains(t, q, "FROM bowling_data", "the scorecard must not define the population")
	// Both groups of a side draw from that side's one squad, which is what the serving
	// path does: it aggregates every group over all of the proposed XI.
	require.Contains(t, q, "FROM t1_squad p")
	require.Contains(t, q, "FROM t2_squad p")
	require.NotContains(t, q, "COUNT(*) AS cnt", "the count aggregate must not come back")
}

// TestWinExport_ExcludesMatchesWithNoRecordedSquad: a match with no squad must be dropped,
// not zero-filled into a side of nobody carrying a real target.
func TestWinExport_ExcludesMatchesWithNoRecordedSquad(t *testing.T) {
	q := winEnhancedQueryForTest()

	require.Contains(t, q, "EXISTS (SELECT 1 FROM match_player mp WHERE mp.match_id = m.match_id "+
		"AND mp.opposition_id = mi.batting_team_opposition_id)")
	require.Contains(t, q, "EXISTS (SELECT 1 FROM match_player mp WHERE mp.match_id = m.match_id "+
		"AND mp.opposition_id = mi.bowling_team_opposition_id)")
}

// TestBuildWinFeatureJoins_ContainsAllFeatureJoins asserts the built JOIN list includes
// LEFT JOIN agg_* and LEFT JOIN top3_* for each of the 8 feature names.
func TestBuildWinFeatureJoins_ContainsAllFeatureJoins(t *testing.T) {
	joins := buildWinFeatureJoins()
	for i, name := range winFeatureCTENames() {
		i, name := i, name // capture range variables
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			n := i + 1
			require.Contains(t, joins, fmt.Sprintf("LEFT JOIN agg_%s a%d ON a%d.match_id = m.match_id", name, n, n))
			require.Contains(
				t,
				joins,
				fmt.Sprintf("LEFT JOIN top3_%s t3a%d ON t3a%d.match_id = m.match_id", name, n, n),
			)
		})
	}
}

// winEnhancedQueryForTest exposes the assembled query to the assertions above.
func winEnhancedQueryForTest() string { return buildWinEnhancedQuery() }
