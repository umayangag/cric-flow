package db

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// UpsertMatch and UpsertMatchTx ran two hand-copied statements, so a column added to one
// reached only half the callers. They now share one statement and one argument list, and
// these tests pin the two properties that made the copies dangerous: every column the
// statement inserts is also refreshed on conflict, and the number of placeholders matches
// the number of arguments.

func TestUpsertMatchSQL_PlaceholderCountMatchesTheArguments(t *testing.T) {
	t.Parallel()

	arguments := upsertMatchArgs(&MatchInsert{MatchID: 1})

	for i := range arguments {
		require.Containsf(t, upsertMatchSQL, fmt.Sprintf("$%d", i+1), "argument %d has no placeholder", i+1)
	}
	require.NotContains(t, upsertMatchSQL, fmt.Sprintf("$%d", len(arguments)+1))
}

func TestUpsertMatchSQL_EveryInsertedColumnIsRefreshedOnConflict(t *testing.T) {
	t.Parallel()

	columns := insertedColumns(t, upsertMatchSQL)

	require.Contains(t, columns, "event_stage")
	require.Contains(t, columns, "event_group")
	for _, column := range columns {
		if column == "match_id" {
			continue // the conflict target is not updated
		}
		require.Containsf(t, upsertMatchSQL, column+" = EXCLUDED."+column, "%s is inserted but never updated", column)
	}
}

// FEAT-09: the match's last day is written beside its first, and refreshed on conflict
// like every other column (the test above), so a re-import fills it.
func TestUpsertMatchSQL_WritesTheMatchEndDate(t *testing.T) {
	t.Parallel()

	arguments := upsertMatchArgs(&MatchInsert{MatchID: 1, MatchDate: "2024-01-01", MatchEndDate: "2024-01-05"})

	require.Contains(t, insertedColumns(t, upsertMatchSQL), "match_end_date")
	require.Contains(t, arguments, "2024-01-05")
}

// insertedColumns reads the column list out of an INSERT ... VALUES statement.
func insertedColumns(t *testing.T, statement string) []string {
	t.Helper()
	start := strings.Index(statement, "(")
	end := strings.Index(statement, ")")
	require.Greater(t, end, start, "statement has no column list")
	names := strings.Split(statement[start+1:end], ",")
	columns := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		require.NotEmpty(t, trimmed)
		columns = append(columns, trimmed)
	}
	return columns
}
