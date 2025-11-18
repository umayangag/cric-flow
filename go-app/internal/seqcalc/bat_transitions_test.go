package seqcalc

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// TestAggregateTransitions_Table follows the gold-standard AAA style using require assertions.
func TestAggregateTransitions_Table(t *testing.T) {
	t.Parallel()

	type arrangeFn func() []bevent
	type assertFn func(t *testing.T, rows []db.BatTransitionRow)

	asOf := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	fmtID := 3

	cases := []struct {
		name    string
		arrange arrangeFn
		assert  assertFn
	}{
		{
			name: "simple innings with wicket transition",
			arrange: func() []bevent {
				// One innings with opener change and a wicket causing a change
				return []bevent{
					{
						MatchID:    1,
						Innings:    1,
						BallSeq:    1,
						Phase:      "powerplay",
						StrikerID:  sql.NullInt64{Int64: 101, Valid: true},
						RunsBatter: 0,
						RunsTotal:  0,
						AsOf:       asOf,
						FormatID:   fmtID,
					},
					{
						MatchID:    1,
						Innings:    1,
						BallSeq:    2,
						Phase:      "powerplay",
						StrikerID:  sql.NullInt64{Int64: 101, Valid: true},
						RunsBatter: 1,
						RunsTotal:  1,
						AsOf:       asOf,
						FormatID:   fmtID,
					},
					// striker change after over strike rotation (simulate change): 101 -> 102
					{
						MatchID:    1,
						Innings:    1,
						BallSeq:    3,
						Phase:      "powerplay",
						StrikerID:  sql.NullInt64{Int64: 102, Valid: true},
						RunsBatter: 0,
						RunsTotal:  0,
						AsOf:       asOf,
						FormatID:   fmtID,
					},
					{
						MatchID:    1,
						Innings:    1,
						BallSeq:    4,
						Phase:      "powerplay",
						StrikerID:  sql.NullInt64{Int64: 102, Valid: true},
						RunsBatter: 4,
						RunsTotal:  4,
						AsOf:       asOf,
						FormatID:   fmtID,
					},
					// wicket: striker 102 out, new batter 103 next ball
					{
						MatchID:     1,
						Innings:     1,
						BallSeq:     5,
						Phase:       "powerplay",
						StrikerID:   sql.NullInt64{Int64: 102, Valid: true},
						RunsBatter:  0,
						RunsTotal:   0,
						WicketKind:  sql.NullString{String: "bowled", Valid: true},
						PlayerOutID: sql.NullInt64{Int64: 102, Valid: true},
						AsOf:        asOf,
						FormatID:    fmtID,
					},
					{
						MatchID:    1,
						Innings:    1,
						BallSeq:    6,
						Phase:      "powerplay",
						StrikerID:  sql.NullInt64{Int64: 103, Valid: true},
						RunsBatter: 6,
						RunsTotal:  6,
						AsOf:       asOf,
						FormatID:   fmtID,
					},
				}
			},
			assert: func(t *testing.T, rows []db.BatTransitionRow) {
				require.Equal(t, 2, len(rows), "expected 2 transition rows")
				// build a map for assertions
				key := func(prev, bat int64) string { return fmt.Sprintf("%d->%d", prev, bat) }
				m := map[string]db.BatTransitionRow{}
				for _, r := range rows {
					m[key(r.PrevBatterID, r.BatterID)] = r
				}
				got := m[key(101, 102)]
				require.Equal(t, 3, got.Balls)
				require.Equal(t, 4, got.Runs)
				require.Equal(t, 1, got.Fours)
				require.Equal(t, 1, got.Dismissals)

				got2 := m[key(102, 103)]
				require.Equal(t, 1, got2.Balls)
				require.Equal(t, 0, got2.Dismissals)
				require.Equal(t, 1, got2.Sixes)
				require.Equal(t, 6, got2.Runs)
			},
		},
		{
			name: "missing striker rows ignored",
			arrange: func() []bevent {
				asOf2 := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
				return []bevent{
					{
						MatchID:    1,
						Innings:    1,
						BallSeq:    1,
						Phase:      "powerplay",
						StrikerID:  sql.NullInt64{Valid: false},
						RunsBatter: 0,
						RunsTotal:  0,
						AsOf:       asOf2,
						FormatID:   fmtID,
					},
					{
						MatchID:    1,
						Innings:    1,
						BallSeq:    2,
						Phase:      "powerplay",
						StrikerID:  sql.NullInt64{Int64: 201, Valid: true},
						RunsBatter: 0,
						RunsTotal:  0,
						AsOf:       asOf2,
						FormatID:   fmtID,
					},
					{
						MatchID:    1,
						Innings:    1,
						BallSeq:    3,
						Phase:      "powerplay",
						StrikerID:  sql.NullInt64{Int64: 202, Valid: true},
						RunsBatter: 1,
						RunsTotal:  1,
						AsOf:       asOf2,
						FormatID:   fmtID,
					},
					{
						MatchID:    1,
						Innings:    1,
						BallSeq:    4,
						Phase:      "powerplay",
						StrikerID:  sql.NullInt64{Int64: 202, Valid: true},
						RunsBatter: 6,
						RunsTotal:  6,
						AsOf:       asOf2,
						FormatID:   fmtID,
					},
				}
			},
			assert: func(t *testing.T, rows []db.BatTransitionRow) {
				require.Equal(t, 1, len(rows))
				r := rows[0]
				require.Equal(t, int64(201), r.PrevBatterID)
				require.Equal(t, int64(202), r.BatterID)
				require.Equal(t, 7, r.Runs)
				require.Equal(t, 0, r.Fours)
				require.Equal(t, 1, r.Sixes)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			seq := tc.arrange()
			// Act
			rows := aggregateTransitions(seq)
			// Assert
			tc.assert(t, rows)
		})
	}
}
