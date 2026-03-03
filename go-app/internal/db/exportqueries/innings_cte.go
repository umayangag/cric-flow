package exportqueries

import "fmt"

// inningsRunsHoldoutCTE returns a CTE snippet that computes total runs per
// match/inning from batting_data for holdout queries that select by match IDs.
// The CTE name is provided by the caller so different queries can choose their
// own alias (e.g. innings_sums, innings_runs_cte).
func inningsRunsHoldoutCTE(cteName string) string {
	return fmt.Sprintf(`%s AS (
		SELECT match_id, inning_number, SUM(runs)::bigint AS total_runs
		FROM batting_data
		WHERE match_id = ANY($1::bigint[])
		GROUP BY match_id, inning_number
	)`, cteName)
}

