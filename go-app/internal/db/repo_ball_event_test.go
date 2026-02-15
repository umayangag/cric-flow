package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// mockPoolAPI adapts pgxmock's pool/tx to the db.PoolIface/db.CopyFromTx for tests.
type mockPoolAPI struct{ p pgxmock.PgxPoolIface }

func (m mockPoolAPI) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := m.p.Exec(ctx, sql, args...)
	return err
}

func (m mockPoolAPI) Query(ctx context.Context, sql string, args ...any) (db.Rows, error) {
	r, err := m.p.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rowsWrap{r}, nil
}

func (m mockPoolAPI) QueryRow(ctx context.Context, sql string, args ...any) db.Row {
	return rowWrap{m.p.QueryRow(ctx, sql, args...)}
}

func (m mockPoolAPI) Begin(ctx context.Context) (db.CopyFromTx, error) {
	// Satisfy ExpectBegin by calling Begin on the mock pool; ignore returned tx.
	if _, err := m.p.Begin(ctx); err != nil {
		return nil, err
	}
	// Return a facade that delegates to the pool for Exec/CopyFrom/Commit etc.
	return mockTxAPI(m), nil
}

type mockTxAPI struct{ p pgxmock.PgxPoolIface }

func (t mockTxAPI) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := t.p.Exec(ctx, sql, args...)
	return err
}

func (t mockTxAPI) Query(ctx context.Context, sql string, args ...any) (db.Rows, error) {
	r, err := t.p.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rowsWrap{r}, nil
}

func (t mockTxAPI) QueryRow(ctx context.Context, sql string, args ...any) db.Row {
	return rowWrap{t.p.QueryRow(ctx, sql, args...)}
}

func (t mockTxAPI) CopyFrom(
	ctx context.Context,
	table pgx.Identifier,
	columns []string,
	src pgx.CopyFromSource,
) (int64, error) {
	return t.p.CopyFrom(ctx, table, columns, src)
}
func (t mockTxAPI) Commit(ctx context.Context) error   { return t.p.Commit(ctx) }
func (t mockTxAPI) Rollback(ctx context.Context) error { return t.p.Rollback(ctx) }

type rowsWrap struct{ pgx.Rows }

func (r rowsWrap) Close() { r.Rows.Close() }

type rowWrap struct{ pgx.Row }

func (r rowWrap) Scan(dest ...any) error { return r.Row.Scan(dest...) }

// TestInsertBallEvents_Scenarios: table-driven tests covering small-batch Exec and bulk CopyFrom paths.
func TestInsertBallEvents_Scenarios(t *testing.T) {
	type arrangeFn func(t *testing.T) (ctx context.Context, mock pgxmock.PgxPoolIface, rows []db.BallEventRow)
	type assertFn func(t *testing.T, mock pgxmock.PgxPoolIface, err error)

	cases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "small batch uses Exec with individual inserts",
			arrange: func(t *testing.T) (context.Context, pgxmock.PgxPoolIface, []db.BallEventRow) {
				ctx := context.Background()
				mock, err := pgxmock.NewPool()
				require.NoError(t, err, "pgxmock.NewPool")
				t.Cleanup(mock.Close)

				// Expect 2 Exec calls (<= small threshold) with any 16 args each
				mock.ExpectExec("INSERT INTO ball_event").
					WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
						pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
						pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
						pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				mock.ExpectExec("INSERT INTO ball_event").
					WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
						pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
						pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
						pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))

				rows := []db.BallEventRow{
					{MatchID: 1, Innings: 1, Over: 1, Ball: 1, BallSeq: 1, IsLegal: true, Phase: "pp"},
					{MatchID: 1, Innings: 1, Over: 1, Ball: 2, BallSeq: 2, IsLegal: true, Phase: "pp"},
				}
				return ctx, mock, rows
			},
			assert: func(t *testing.T, mock pgxmock.PgxPoolIface, err error) {
				require.NoError(t, err, "InsertBallEvents small batch")
				require.NoError(t, mock.ExpectationsWereMet())
			},
		},
		{
			name: "bulk path uses CopyFrom and staged insert",
			arrange: func(t *testing.T) (context.Context, pgxmock.PgxPoolIface, []db.BallEventRow) {
				ctx := context.Background()
				mock, err := pgxmock.NewPool()
				require.NoError(t, err, "pgxmock.NewPool")
				t.Cleanup(mock.Close)

				// Bulk path expectations
				mock.ExpectBegin()
				mock.ExpectExec("DROP TABLE IF EXISTS ball_event_stage").
					WillReturnResult(pgxmock.NewResult("DROP TABLE", 0))
				mock.ExpectExec("CREATE TEMP TABLE ball_event_stage ON COMMIT DROP AS").
					WillReturnResult(pgxmock.NewResult("CREATE TABLE", 0))
				mock.ExpectCopyFrom(pgx.Identifier{"ball_event_stage"}, []string{"match_id", "innings", "over", "ball", "ball_seq", "is_legal", "phase", "striker_id", "non_striker_id", "bowler_id", "runs_batter", "runs_extras", "runs_total", "extras_kind", "wicket_kind", "player_out_id"}).
					WillReturnResult(10)
				mock.ExpectExec("INSERT INTO ball_event").WillReturnResult(pgxmock.NewResult("INSERT", 10))
				mock.ExpectCommit()

				// len(rows)=9 to exceed smallBatchThreshold(8)
				rows := make([]db.BallEventRow, 9)
				for i := 0; i < 9; i++ {
					rows[i] = db.BallEventRow{
						MatchID: 1,
						Innings: 1,
						Over:    1 + i/6,
						Ball:    1 + i%6,
						BallSeq: 1 + i,
						IsLegal: true,
						Phase:   "pp",
					}
				}
				return ctx, mock, rows
			},
			assert: func(t *testing.T, mock pgxmock.PgxPoolIface, err error) {
				require.NoError(t, err, "InsertBallEvents bulk")
				require.NoError(t, mock.ExpectationsWereMet())
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc := tc
			// Arrange
			ctx, mock, rows := tc.arrange(t)
			db.SetPoolAPI(mockPoolAPI{p: mock})
			t.Cleanup(func() { db.SetPoolAPI(nil) })
			// Act
			err := db.InsertBallEvents(ctx, rows)
			// Assert
			tc.assert(t, mock, err)
		})
	}
}
