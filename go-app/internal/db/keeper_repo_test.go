package db_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

func TestCountPlayersByLowerName(t *testing.T) {
	ctx := context.Background()

	mock, err := pgxmock.NewPool()
	require.NoError(t, err, "pgxmock")
	t.Cleanup(mock.Close)

	// Inject our mock DB
	db.SetDB(mockDB{pool: mock})

	// Happy and error paths table
	testCases := []struct {
		name      string
		arg       string
		setup     func()
		wantCount int64
		wantErr   bool
	}{
		{
			name: "happy -> count 2",
			arg:  "john doe",
			setup: func() {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(1) FROM player WHERE lower(player_name) = $1`)).
					WithArgs("john doe").
					WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(2)))
			},
			wantCount: 2,
		},
		{
			name: "not found -> count 0",
			arg:  "jane doe",
			setup: func() {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(1) FROM player WHERE lower(player_name) = $1`)).
					WithArgs("jane doe").
					WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))
			},
			wantCount: 0,
		},
		{
			name: "db error",
			arg:  "bad name",
			setup: func() {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(1) FROM player WHERE lower(player_name) = $1`)).
					WithArgs("bad name").
					WillReturnError(errors.New("boom"))
			},
			wantErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tc.setup()
			// Act
			got, err := db.CountPlayersByLowerName(ctx, tc.arg)
			// Assert
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantCount, got)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSetIsWicketKeeperByLowerName(t *testing.T) {
	ctx := context.Background()

	type arrangeFn func(t *testing.T) (pgxmock.PgxPoolIface, int, string)
	type assertFn func(t *testing.T, n int64, err error, mock pgxmock.PgxPoolIface)

	testCases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "happy -> multiple affected",
			arrange: func(t *testing.T) (pgxmock.PgxPoolIface, int, string) {
				mock, err := pgxmock.NewPool()
				require.NoError(t, err)
				t.Cleanup(mock.Close)
				db.SetDB(mockDB{pool: mock})
				mock.ExpectQuery(regexp.QuoteMeta(`UPDATE player SET is_wicket_keeper = $1 WHERE lower(player_name) = $2 RETURNING 1`)).
					WithArgs(1, "kumar sangakkara").
					WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1).AddRow(1).AddRow(1))
				return mock, 1, "kumar sangakkara"
			},
			assert: func(t *testing.T, n int64, err error, mock pgxmock.PgxPoolIface) {
				require.NoError(t, err)
				require.Equal(t, int64(3), n)
				require.NoError(t, mock.ExpectationsWereMet())
			},
		},
		{
			name: "db error",
			arrange: func(t *testing.T) (pgxmock.PgxPoolIface, int, string) {
				mock, err := pgxmock.NewPool()
				require.NoError(t, err)
				t.Cleanup(mock.Close)
				db.SetDB(mockDB{pool: mock})
				mock.ExpectQuery(regexp.QuoteMeta(`UPDATE player SET is_wicket_keeper = $1 WHERE lower(player_name) = $2 RETURNING 1`)).
					WithArgs(0, "foo bar").
					WillReturnError(errors.New("fail"))
				return mock, 0, "foo bar"
			},
			assert: func(t *testing.T, _ int64, err error, mock pgxmock.PgxPoolIface) {
				require.Error(t, err)
				require.NoError(t, mock.ExpectationsWereMet())
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mock, flag, name := tc.arrange(t)
			// Act
			n, err := db.SetIsWicketKeeperByLowerName(ctx, flag, name)
			// Assert
			tc.assert(t, n, err, mock)
		})
	}
}

func TestZeroKeepersExcept(t *testing.T) {
	ctx := context.Background()

	type arrangeFn func(t *testing.T) (pgxmock.PgxPoolIface, []string)
	type assertFn func(t *testing.T, n int64, err error, mock pgxmock.PgxPoolIface)

	testCases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "empty list -> returns error",
			arrange: func(t *testing.T) (pgxmock.PgxPoolIface, []string) {
				mock, err := pgxmock.NewPool()
				require.NoError(t, err)
				t.Cleanup(mock.Close)
				db.SetDB(mockDB{pool: mock})
				return mock, nil
			},
			assert: func(t *testing.T, n int64, err error, mock pgxmock.PgxPoolIface) {
				require.Error(t, err)
				require.Equal(t, int64(0), n)
				require.NoError(t, mock.ExpectationsWereMet())
			},
		},
		{
			name: "non-empty -> returning rows counted",
			arrange: func(t *testing.T) (pgxmock.PgxPoolIface, []string) {
				mock, err := pgxmock.NewPool()
				require.NoError(t, err)
				t.Cleanup(mock.Close)
				db.SetDB(mockDB{pool: mock})
				mock.ExpectQuery(regexp.QuoteMeta(`UPDATE player SET is_wicket_keeper = 0 WHERE lower(player_name) NOT IN ($1,$2) RETURNING 1`)).
					WithArgs("adam gilchrist", "ms dhoni").
					WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1).AddRow(1).AddRow(1).AddRow(1))
				return mock, []string{"adam gilchrist", "ms dhoni"}
			},
			assert: func(t *testing.T, n int64, err error, mock pgxmock.PgxPoolIface) {
				require.NoError(t, err)
				require.Equal(t, int64(4), n)
				require.NoError(t, mock.ExpectationsWereMet())
			},
		},
		{
			name: "db error on returning path",
			arrange: func(t *testing.T) (pgxmock.PgxPoolIface, []string) {
				mock, err := pgxmock.NewPool()
				require.NoError(t, err)
				t.Cleanup(mock.Close)
				db.SetDB(mockDB{pool: mock})
				mock.ExpectQuery(regexp.QuoteMeta(`UPDATE player SET is_wicket_keeper = 0 WHERE lower(player_name) NOT IN ($1) RETURNING 1`)).
					WithArgs("only one").
					WillReturnError(errors.New("boom"))
				return mock, []string{"only one"}
			},
			assert: func(t *testing.T, _ int64, err error, mock pgxmock.PgxPoolIface) {
				require.Error(t, err)
				require.NoError(t, mock.ExpectationsWereMet())
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mock, keepers := tc.arrange(t)
			// Act
			n, err := db.ZeroKeepersExcept(ctx, keepers)
			// Assert
			tc.assert(t, n, err, mock)
		})
	}
}
