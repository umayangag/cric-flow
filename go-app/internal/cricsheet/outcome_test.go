package cricsheet_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// This file pins IMPORT-02: Cricsheet writes a tie-breaker win as
// {result: "tie", eliminator: "<team>"} with no winner, and the importer used to read only
// winner, so a super-over win landed as no winner at all -- the same record as an abandoned
// match -- and the rating pass left it out. The outcome is decoded whole, the side the
// match went to is one rule (Outcome.WinningTeam), and the result is kept beside it so a
// tie-breaker win stays distinguishable from an outright one.

func TestParse_Outcome_Decoded(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		outcome string
		want    cricsheet.Outcome
	}{
		{
			name:    "an outright win carries the winner and the margin",
			outcome: `{"winner":"Alpha","by":{"runs":12}}`,
			want:    cricsheet.Outcome{Winner: "Alpha", By: &cricsheet.OutcomeBy{Runs: intPtr(12)}},
		},
		{
			name:    "a tie decided by a super over names the eliminator, not a winner",
			outcome: `{"result":"tie","eliminator":"Beta"}`,
			want:    cricsheet.Outcome{Result: "tie", Eliminator: "Beta"},
		},
		{
			name:    "a tie decided by a bowl-out names the bowl_out side",
			outcome: `{"result":"tie","bowl_out":"Alpha"}`,
			want:    cricsheet.Outcome{Result: "tie", BowlOut: "Alpha"},
		},
		{
			name:    "a rain-adjusted win carries the method",
			outcome: `{"winner":"Alpha","by":{"wickets":3},"method":"D/L"}`,
			want:    cricsheet.Outcome{Winner: "Alpha", By: &cricsheet.OutcomeBy{Wickets: intPtr(3)}, Method: "D/L"},
		},
		{
			name:    "a no-result carries only the result",
			outcome: `{"result":"no result"}`,
			want:    cricsheet.Outcome{Result: "no result"},
		},
		{
			name:    "a draw carries only the result",
			outcome: `{"result":"draw"}`,
			want:    cricsheet.Outcome{Result: "draw"},
		},
		{
			name: "the archive's longest method is read whole",
			// One file carries this eighteen-character method; a varchar(16) column would
			// have refused the whole file, which is why result_method is wider.
			outcome: `{"winner":"Alpha","method":"Lost fewer wickets"}`,
			want:    cricsheet.Outcome{Winner: "Alpha", Method: "Lost fewer wickets"},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			file := `{"info":{"dates":["2024-01-02"],"teams":["Alpha","Beta"],"outcome":` + tc.outcome + `},"innings":[]}`

			// Act
			match, err := cricsheet.Parse(strings.NewReader(file))

			// Assert
			require.NoError(t, err)
			require.NotNil(t, match.Info.Outcome)
			assert.Equal(t, tc.want, *match.Info.Outcome)
		})
	}
}

func TestOutcome_WinningTeam(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		outcome *cricsheet.Outcome
		want    string
	}{
		{
			name:    "no outcome at all is no winner",
			outcome: nil,
			want:    "",
		},
		{
			name:    "an outright winner is the winner",
			outcome: &cricsheet.Outcome{Winner: "Alpha", By: &cricsheet.OutcomeBy{Runs: intPtr(12)}},
			want:    "Alpha",
		},
		{
			name:    "a tie decided by a super over goes to the eliminator",
			outcome: &cricsheet.Outcome{Result: "tie", Eliminator: "Beta"},
			want:    "Beta",
		},
		{
			name:    "a tie decided by a bowl-out goes to the bowl_out side",
			outcome: &cricsheet.Outcome{Result: "tie", BowlOut: "Alpha"},
			want:    "Alpha",
		},
		{
			name:    "a tie nobody broke has no winner",
			outcome: &cricsheet.Outcome{Result: "tie"},
			want:    "",
		},
		{
			name:    "a no-result has no winner",
			outcome: &cricsheet.Outcome{Result: "no result"},
			want:    "",
		},
		{
			name:    "a draw has no winner",
			outcome: &cricsheet.Outcome{Result: "draw"},
			want:    "",
		},
		{
			name:    "an awarded match goes to the side it was awarded to",
			outcome: &cricsheet.Outcome{Winner: "Beta", Method: "Awarded"},
			want:    "Beta",
		},
		{
			name:    "surrounding space on a side is trimmed, as the winner always was",
			outcome: &cricsheet.Outcome{Result: "tie", Eliminator: " Beta "},
			want:    "Beta",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Act
			got := tc.outcome.WinningTeam()

			// Assert
			assert.Equal(t, tc.want, got)
		})
	}
}

// matchUpsertSpyTx captures the argument list of the match upsert, which is where the
// result and the method reach the database.
type matchUpsertSpyTx struct {
	matchArgs []any
}

// Positions in upsertMatchSQL's argument list: result and result_method follow the
// outcome margin.
const (
	matchArgResult       = 13
	matchArgResultMethod = 14
	matchArgCount        = 22
)

func (s *matchUpsertSpyTx) Exec(_ context.Context, sql string, args ...any) error {
	if strings.Contains(sql, "INSERT INTO match (") {
		s.matchArgs = args
	}
	return nil
}

func (s *matchUpsertSpyTx) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nopRows{}, nil
}

func (s *matchUpsertSpyTx) QueryRow(_ context.Context, _ string, _ ...any) db.Row { return nopRow{} }

func (s *matchUpsertSpyTx) CopyFrom(
	_ context.Context,
	_ pgx.Identifier,
	_ []string,
	src pgx.CopyFromSource,
) (int64, error) {
	n := int64(0)
	for src.Next() {
		n++
	}
	return n, src.Err()
}

func (s *matchUpsertSpyTx) Commit(_ context.Context) error   { return nil }
func (s *matchUpsertSpyTx) Rollback(_ context.Context) error { return nil }

func TestImportMatchFile_StoresTheResultAndMethodBesideTheWinner(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	testCases := []struct {
		name       string
		outcome    string
		wantResult *string
		wantMethod *string
	}{
		{
			name:       "a tie decided by a super over keeps its result",
			outcome:    `{"result":"tie","eliminator":"Beta"}`,
			wantResult: strPtr("tie"),
			wantMethod: nil,
		},
		{
			name:       "a no-result keeps its result and stays without a method",
			outcome:    `{"result":"no result"}`,
			wantResult: strPtr("no result"),
			wantMethod: nil,
		},
		{
			name:       "an outright win has no result and keeps its method",
			outcome:    `{"winner":"Alpha","by":{"runs":12},"method":"D/L"}`,
			wantResult: nil,
			wantMethod: strPtr("D/L"),
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			prevPool := db.PoolAPI
			db.SetPoolAPI(nopPool{})
			t.Cleanup(func() { db.SetPoolAPI(prevPool) })
			spy := &matchUpsertSpyTx{}
			cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
				return inner(ctx, spy)
			})
			t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })
			file := writeTempJSON(t, t.TempDir(), "9000020.json", matchFileWithOutcomeJSON(tc.outcome,
				inningsOf("Alpha", "A1", "A2", "B1", []int{4, 1, 6}, ""),
				inningsOf("Beta", "B2", "B3", "A1", []int{4, 6, 1}, ""),
			))

			// Act
			err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

			// Assert
			require.NoError(t, err)
			require.Len(t, spy.matchArgs, matchArgCount, "the match upsert carries result and result_method")
			assert.Equal(t, tc.wantResult, spy.matchArgs[matchArgResult])
			assert.Equal(t, tc.wantMethod, spy.matchArgs[matchArgResultMethod])
		})
	}
}

func intPtr(v int) *int { return &v }

func strPtr(s string) *string { return &s }
