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

func TestReplaceMatchPlayersTx_WithRows_DeletesThenInsertsEveryRow(t *testing.T) {
	// Arrange
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	mock.ExpectExec("DELETE FROM match_player").
		WithArgs(int64(7)).
		WillReturnResult(pgxmock.NewResult("DELETE", 2))
	mock.ExpectExec("INSERT INTO match_player").
		WithArgs(int64(7), int64(1), int64(10), int64(7), int64(2), int64(20)).
		WillReturnResult(pgxmock.NewResult("INSERT", 2))
	rows := []db.MatchPlayer{
		{MatchID: 7, PlayerID: 1, OppositionID: 10},
		{MatchID: 7, PlayerID: 2, OppositionID: 20},
	}

	// Act
	err = db.ReplaceMatchPlayersTx(context.Background(), mockTxAPI{mock}, 7, rows)

	// Assert
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceMatchPlayersTx_NoRows_StillClearsTheOldSquad(t *testing.T) {
	// Arrange
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	// A re-import of a file that lost its info.players must not leave the previous
	// squad in place; the delete runs whether or not there is anything to write.
	mock.ExpectExec("DELETE FROM match_player").
		WithArgs(int64(7)).
		WillReturnResult(pgxmock.NewResult("DELETE", 22))

	// Act
	err = db.ReplaceMatchPlayersTx(context.Background(), mockTxAPI{mock}, 7, nil)

	// Assert
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceMatchPlayersTx_DeleteFails_ReturnsErrorAndSkipsInsert(t *testing.T) {
	// Arrange
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	sentinel := errors.New("delete refused")
	mock.ExpectExec("DELETE FROM match_player").WithArgs(int64(7)).WillReturnError(sentinel)

	// Act
	err = db.ReplaceMatchPlayersTx(
		context.Background(),
		mockTxAPI{mock},
		7,
		[]db.MatchPlayer{{MatchID: 7, PlayerID: 1, OppositionID: 10}},
	)

	// Assert
	require.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "match 7")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceMatchPlayersTx_InsertFails_ReturnsErrorNamingTheRowCount(t *testing.T) {
	// Arrange
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	sentinel := errors.New("insert refused")
	mock.ExpectExec("DELETE FROM match_player").
		WithArgs(int64(7)).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))
	mock.ExpectExec("INSERT INTO match_player").
		WithArgs(int64(7), int64(1), int64(10)).
		WillReturnError(sentinel)

	// Act
	err = db.ReplaceMatchPlayersTx(
		context.Background(),
		mockTxAPI{mock},
		7,
		[]db.MatchPlayer{{MatchID: 7, PlayerID: 1, OppositionID: 10}},
	)

	// Assert
	require.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "insert 1 match_player rows")
	require.NoError(t, mock.ExpectationsWereMet())
}
