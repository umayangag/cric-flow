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
	require.False(t, strings.HasSuffix(strings.TrimSpace(cols), ","), "last select column group must not end with comma")
}

// TestBuildWinAggAndTop3CTEs_ContainsAllFeatureCTEs asserts the built CTE string includes
// agg_* and top3_* CTEs for each of the 8 win feature names (t1_bat_cons, t1_bowl_cons, ...).
func TestBuildWinAggAndTop3CTEs_ContainsAllFeatureCTEs(t *testing.T) {
	ctes := buildWinAggAndTop3CTEs()
	for _, name := range winFeatureCTENames {
		t.Run(name, func(t *testing.T) {
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
// include all 8 feature aliases (a1..a8) and top3 aliases (t3a1..t3a8) with the seven stats.
func TestBuildWinFeatureSelectColumns_ContainsAllFeatureColumns(t *testing.T) {
	cols := buildWinFeatureSelectColumns()
	for n := 1; n <= 8; n++ {
		require.Contains(t, cols, fmt.Sprintf("a%d.", n), "columns must reference agg alias a%d", n)
		require.Contains(t, cols, fmt.Sprintf("t3a%d.", n), "columns must reference top3 alias t3a%d", n)
	}
	require.Contains(t, cols, "top3_mean", "columns must include top3_mean")
	require.Contains(t, cols, ".cnt, 0)", "columns must include count stat")
}

// TestBuildWinFeatureJoins_ContainsAllFeatureJoins asserts the built JOIN list includes
// LEFT JOIN agg_* and LEFT JOIN top3_* for each of the 8 feature names.
func TestBuildWinFeatureJoins_ContainsAllFeatureJoins(t *testing.T) {
	joins := buildWinFeatureJoins()
	for i, name := range winFeatureCTENames {
		n := i + 1
		t.Run(name, func(t *testing.T) {
			require.Contains(t, joins, fmt.Sprintf("LEFT JOIN agg_%s a%d ON a%d.match_id = m.match_id", name, n, n))
			require.Contains(t, joins, fmt.Sprintf("LEFT JOIN top3_%s t3a%d ON t3a%d.match_id = m.match_id", name, n, n))
		})
	}
}
