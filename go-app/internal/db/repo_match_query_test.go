package db

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgxmock "github.com/pashagolub/pgxmock/v4"
)

func TestGetMatchDate(t *testing.T) {
	ctx := context.Background()

	t.Run("happy -> returns time", func(t *testing.T) {
		mock, _ := pgxmock.NewPool()
		defer mock.Close()
		SetDB(mockDB{pool: mock})
		Pool = &pgxpool.Pool{}
		ts := time.Date(2020, 5, 17, 0, 0, 0, 0, time.UTC)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT date FROM match_details WHERE match_id = $1`)).
			WithArgs(int64(42)).
			WillReturnRows(pgxmock.NewRows([]string{"date"}).AddRow(ts))
		got, err := GetMatchDate(ctx, 42)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got == nil || !got.Equal(ts) {
			t.Fatalf("date mismatch: got %v want %v", got, ts)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("null -> returns nil", func(t *testing.T) {
		mock, _ := pgxmock.NewPool()
		defer mock.Close()
		SetDB(mockDB{pool: mock})
		Pool = &pgxpool.Pool{}
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT date FROM match_details WHERE match_id = $1`)).
			WithArgs(int64(7)).
			WillReturnRows(pgxmock.NewRows([]string{"date"}).AddRow(nil))
		got, err := GetMatchDate(ctx, 7)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil date, got %v", got)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("db error", func(t *testing.T) {
		mock, _ := pgxmock.NewPool()
		defer mock.Close()
		SetDB(mockDB{pool: mock})
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT date FROM match_details WHERE match_id = $1`)).
			WithArgs(int64(99)).
			WillReturnError(errors.New("boom"))
		if _, err := GetMatchDate(ctx, 99); err == nil {
			t.Fatalf("expected error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})
}
