package db

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
)

// mockDB adapts pgxmock pool to our DB interface for tests.
type mockDB struct{ pool pgxmock.PgxPoolIface }

type mockRows struct{ rows pgx.Rows }

func (r mockRows) Next() bool             { return r.rows.Next() }
func (r mockRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r mockRows) Close()                 { r.rows.Close() }

type mockRow struct{ row pgx.Row }

func (r mockRow) Scan(dest ...any) error { return r.row.Scan(dest...) }

func (m mockDB) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := m.pool.Exec(ctx, sql, args...)
	return err
}

func (m mockDB) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	r, err := m.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return mockRows{rows: r}, nil
}

func (m mockDB) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return mockRow{row: m.pool.QueryRow(ctx, sql, args...)}
}

func (m mockDB) Begin(_ context.Context) (Tx, error) {
	return nil, errors.New("not implemented in tests")
}

func TestCountPlayersByLowerName(t *testing.T) {
	ctx := context.Background()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	// Inject our mock DB
	SetDB(mockDB{pool: mock})

	// Happy and error paths table
	tests := []struct {
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

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.setup()
			got, err := CountPlayersByLowerName(ctx, tc.arg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error mismatch: %v", err)
			}
			if !tc.wantErr && got != tc.wantCount {
				t.Fatalf("count mismatch: got %d want %d", got, tc.wantCount)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet expectations: %v", err)
			}
		})
	}
}

func TestSetIsWicketKeeperByLowerName(t *testing.T) {
	ctx := context.Background()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	SetDB(mockDB{pool: mock})

	t.Run("happy -> multiple affected", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE player SET is_wicket_keeper = $1 WHERE lower(player_name) = $2 RETURNING 1`)).
			WithArgs(1, "kumar sangakkara").
			WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1).AddRow(1).AddRow(1))
		n, err := SetIsWicketKeeperByLowerName(ctx, 1, "kumar sangakkara")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if n != 3 {
			t.Fatalf("affected mismatch: got %d want %d", n, 3)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("db error", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE player SET is_wicket_keeper = $1 WHERE lower(player_name) = $2 RETURNING 1`)).
			WithArgs(0, "foo bar").
			WillReturnError(errors.New("fail"))
		_, err := SetIsWicketKeeperByLowerName(ctx, 0, "foo bar")
		if err == nil {
			t.Fatalf("expected error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})
}

func TestZeroKeepersExcept(t *testing.T) {
	ctx := context.Background()

	t.Run("empty list -> exec then count", func(t *testing.T) {
		mock, _ := pgxmock.NewPool()
		defer mock.Close()
		SetDB(mockDB{pool: mock})
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE player SET is_wicket_keeper = 0`)).
			WillReturnResult(pgxmock.NewResult("UPDATE", 10))
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(1) FROM player`)).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(10)))
		n, err := ZeroKeepersExcept(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if n != 10 {
			t.Fatalf("count mismatch: got %d want %d", n, 10)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("non-empty -> returning rows counted", func(t *testing.T) {
		mock, _ := pgxmock.NewPool()
		defer mock.Close()
		SetDB(mockDB{pool: mock})
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE player SET is_wicket_keeper = 0 WHERE lower(player_name) NOT IN ($1,$2) RETURNING 1`)).
			WithArgs("adam gilchrist", "ms dhoni").
			WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1).AddRow(1).AddRow(1).AddRow(1))
		n, err := ZeroKeepersExcept(ctx, []string{"adam gilchrist", "ms dhoni"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if n != 4 {
			t.Fatalf("affected mismatch: got %d want %d", n, 4)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("db error on returning path", func(t *testing.T) {
		mock, _ := pgxmock.NewPool()
		defer mock.Close()
		SetDB(mockDB{pool: mock})
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE player SET is_wicket_keeper = 0 WHERE lower(player_name) NOT IN ($1) RETURNING 1`)).
			WithArgs("only one").
			WillReturnError(errors.New("boom"))
		if _, err := ZeroKeepersExcept(ctx, []string{"only one"}); err == nil {
			t.Fatalf("expected error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})
}
