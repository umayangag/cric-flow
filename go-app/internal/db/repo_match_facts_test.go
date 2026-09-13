package db_test

import (
	"context"
	"errors"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// matchFactTables is the list DeleteMatchFactsTx is expected to clear, in the order it
// clears them. Written out here rather than read from the package: this test's job is to
// fail when a table is dropped from that list, and a test that shares the list cannot.
var matchFactTables = []string{
	"ball_event",
	"fielding_event",
	"batting_data",
	"bowling_data",
	"fielding_data",
	"match_inning",
}

// expectDeletesFailingAt sets up one DELETE expectation per fact table up to and including
// failingTable, the last of which returns failure. Nothing is expected after it: a delete
// that fails must stop the import rather than carry on clearing the rest of the match.
func expectDeletesFailingAt(
	t *testing.T,
	mock pgxmock.PgxPoolIface,
	failingTable string,
	failure error,
) {
	t.Helper()
	for i := range matchFactTables {
		expectation := mock.ExpectExec("DELETE FROM " + matchFactTables[i] + " WHERE match_id").
			WithArgs(int64(7))
		if matchFactTables[i] == failingTable {
			expectation.WillReturnError(failure)
			return
		}
		expectation.WillReturnResult(pgxmock.NewResult("DELETE", 0))
	}
	t.Fatalf("no fact table named %q; the list this test asserts is out of date", failingTable)
}

func TestDeleteMatchFactsTx_EveryFactTable_IsClearedForThatMatch(t *testing.T) {
	// Arrange
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	for i := range matchFactTables {
		mock.ExpectExec("DELETE FROM " + matchFactTables[i] + " WHERE match_id").
			WithArgs(int64(7)).
			WillReturnResult(pgxmock.NewResult("DELETE", 3))
	}

	// Act
	err = db.DeleteMatchFactsTx(context.Background(), mockTxAPI{mock}, 7)

	// Assert
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteMatchFactsTx_DeleteFails_ReturnsErrorNamingTheTableAndMatch(t *testing.T) {
	testCases := []struct {
		name          string
		failingTable  string
		expectedInErr string
	}{
		{
			name:          "the first table stops the import before anything is written",
			failingTable:  "ball_event",
			expectedInErr: "delete ball_event for match 7",
		},
		{
			name:          "a later table stops it just the same",
			failingTable:  "match_inning",
			expectedInErr: "delete match_inning for match 7",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mock, err := pgxmock.NewPool()
			require.NoError(t, err)
			defer mock.Close()
			sentinel := errors.New("delete refused")
			expectDeletesFailingAt(t, mock, tc.failingTable, sentinel)

			// Act
			err = db.DeleteMatchFactsTx(context.Background(), mockTxAPI{mock}, 7)

			// Assert
			require.ErrorIs(t, err, sentinel)
			assert.Contains(t, err.Error(), tc.expectedInErr)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
