package db_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// TestGetMatchDate_Table refactors scenarios into table-driven subtests with AAA and require.
func TestGetMatchDate_Table(t *testing.T) {
	ctx := context.Background()

	type arrangeFn func(t *testing.T) (mock pgxmock.PgxPoolIface, matchID int64)
	type assertFn func(t *testing.T, got *time.Time, err error, mock pgxmock.PgxPoolIface)

	cases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "happy -> returns time",
			arrange: func(t *testing.T) (pgxmock.PgxPoolIface, int64) {
				mock, err := pgxmock.NewPool()
				require.NoError(t, err)
				t.Cleanup(mock.Close)
				db.SetDB(mockDB{pool: mock})
				// Some code paths check db.Pool not nil; maintain previous pattern
				db.Pool = &pgxpool.Pool{}
				ts := time.Date(2020, 5, 17, 0, 0, 0, 0, time.UTC)
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT date FROM match_details WHERE match_id = $1`)).
					WithArgs(int64(42)).
					WillReturnRows(pgxmock.NewRows([]string{"date"}).AddRow(ts))
				return mock, 42
			},
			assert: func(t *testing.T, got *time.Time, err error, mock pgxmock.PgxPoolIface) {
				require.NoError(t, err)
				require.NotNil(t, got)
				require.True(t, got.Equal(time.Date(2020, 5, 17, 0, 0, 0, 0, time.UTC)))
				require.NoError(t, mock.ExpectationsWereMet())
			},
		},
		{
			name: "null -> returns nil",
			arrange: func(t *testing.T) (pgxmock.PgxPoolIface, int64) {
				mock, err := pgxmock.NewPool()
				require.NoError(t, err)
				t.Cleanup(mock.Close)
				db.SetDB(mockDB{pool: mock})
				db.Pool = &pgxpool.Pool{}
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT date FROM match_details WHERE match_id = $1`)).
					WithArgs(int64(7)).
					WillReturnRows(pgxmock.NewRows([]string{"date"}).AddRow(nil))
				return mock, 7
			},
			assert: func(t *testing.T, got *time.Time, err error, mock pgxmock.PgxPoolIface) {
				require.NoError(t, err)
				require.Nil(t, got)
				require.NoError(t, mock.ExpectationsWereMet())
			},
		},
		{
			name: "db error",
			arrange: func(t *testing.T) (pgxmock.PgxPoolIface, int64) {
				mock, err := pgxmock.NewPool()
				require.NoError(t, err)
				t.Cleanup(mock.Close)
				db.SetDB(mockDB{pool: mock})
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT date FROM match_details WHERE match_id = $1`)).
					WithArgs(int64(99)).
					WillReturnError(errors.New("boom"))
				return mock, 99
			},
			assert: func(t *testing.T, _ *time.Time, err error, mock pgxmock.PgxPoolIface) {
				require.Error(t, err)
				require.NoError(t, mock.ExpectationsWereMet())
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mock, matchID := tc.arrange(t)
			// Act
			got, err := db.GetMatchDate(ctx, matchID)
			// Assert
			tc.assert(t, got, err, mock)
		})
	}
}
