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

// insertedColumns reads the column list out of an INSERT ... VALUES statement.
func insertedColumns(t *testing.T, statement string) []string {
	t.Helper()
	open := strings.Index(statement, "(")
	close := strings.Index(statement, ")")
	require.Greater(t, close, open, "statement has no column list")
	var columns []string
	for _, name := range strings.Split(statement[open+1:close], ",") {
		trimmed := strings.TrimSpace(name)
		require.NotEmpty(t, trimmed)
		columns = append(columns, trimmed)
	}
	return columns
}
