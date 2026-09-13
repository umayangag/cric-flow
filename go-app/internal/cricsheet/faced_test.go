package cricsheet_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// This file pins IMPORT-05. A ball faced and a ball bowled are two counts: the batter
// faces every delivery but a wide, the bowler is credited with every delivery but a wide
// or a no-ball. The importer used one rule -- the bowler's -- for both, so every no-ball a
// batter faced was missing from his balls and his strike rate. The fixture is
// extrasMatchJSON (extras_test.go): A1 is on strike for a four, a no-ball with four
// leg-byes, four byes, a wide and a single beside a penalty -- five deliveries, of which
// he faced four and B1 bowled three.

func TestDelivery_FacedByBatterAndIsLegal_AreTwoCounts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		extras    cricsheet.ExtrasBreakdown
		wantFaced bool
		wantLegal bool
	}{
		{
			name:      "a delivery with no extras is faced and is one of the six",
			extras:    cricsheet.ExtrasBreakdown{},
			wantFaced: true,
			wantLegal: true,
		},
		{
			name:      "a wide is neither faced nor one of the six",
			extras:    cricsheet.ExtrasBreakdown{Wides: 1},
			wantFaced: false,
			wantLegal: false,
		},
		{
			name:      "a no-ball is faced and is not one of the six",
			extras:    cricsheet.ExtrasBreakdown{NoBalls: 1},
			wantFaced: true,
			wantLegal: false,
		},
		{
			name:      "byes are faced and one of the six",
			extras:    cricsheet.ExtrasBreakdown{Byes: 4},
			wantFaced: true,
			wantLegal: true,
		},
		{
			name:      "leg-byes are faced and one of the six",
			extras:    cricsheet.ExtrasBreakdown{LegByes: 1},
			wantFaced: true,
			wantLegal: true,
		},
		{
			name:      "a penalty changes neither count",
			extras:    cricsheet.ExtrasBreakdown{Penalty: 5},
			wantFaced: true,
			wantLegal: true,
		},
		{
			name:      "a no-ball with four leg-byes is faced and not one of the six",
			extras:    cricsheet.ExtrasBreakdown{NoBalls: 1, LegByes: 4},
			wantFaced: true,
			wantLegal: false,
		},
		{
			name:      "a penalty beside a wide is still a wide",
			extras:    cricsheet.ExtrasBreakdown{Wides: 1, Penalty: 5},
			wantFaced: false,
			wantLegal: false,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			delivery := cricsheet.Delivery{Extras: tc.extras}

			gotFaced := delivery.FacedByBatter()
			gotLegal := delivery.IsLegal()

			assert.Equal(t, tc.wantFaced, gotFaced, "faced by the batter")
			assert.Equal(t, tc.wantLegal, gotLegal, "one of the bowler's six")
		})
	}
}

func TestBuildBallEventRows_IsLegal_IsTheBowlersCount(t *testing.T) {
	t.Parallel()
	// Arrange
	match, err := cricsheet.Parse(strings.NewReader(extrasMatchJSON))
	require.NoError(t, err)
	identity := playerIDsByName{"A1": 1, "A2": 2, "B1": 11, "B2": 12, "B3": 13}

	// Act
	rows, err := cricsheet.BuildBallEventRows(context.Background(), identity, match, 1, 9000011)

	// Assert
	require.NoError(t, err)
	require.Len(t, rows, 13)
	testCases := []struct {
		name        string
		row         db.BallEventRow
		wantLegal   bool
		wantBallSeq int
	}{
		{name: "a four is one of the six", row: rows[0], wantLegal: true, wantBallSeq: 1},
		{name: "a no-ball is not, though the batter faced it", row: rows[1], wantLegal: false, wantBallSeq: 1},
		{name: "four byes are", row: rows[2], wantLegal: true, wantBallSeq: 2},
		{name: "a wide is not", row: rows[3], wantLegal: false, wantBallSeq: 2},
		{name: "a penalty beside a single is", row: rows[4], wantLegal: true, wantBallSeq: 3},
		{name: "the over's last ball is its fifth legal one", row: rows[6], wantLegal: true, wantBallSeq: 5},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.wantLegal, tc.row.IsLegal, "is_legal")
			assert.Equal(t, tc.wantBallSeq, tc.row.BallSeq, "ball_seq advances on legal deliveries only")
		})
	}
}

func TestImportMatchFile_ExtrasByKind_CountsTheBatterEveryBallButAWide(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn).
	// Arrange
	ctx := context.Background()
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })
	spy := newExtrasSpyTx()
	cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
		return inner(ctx, spy)
	})
	t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })
	file := writeTempJSON(t, t.TempDir(), "9000011.json", extrasMatchJSON)

	// Act
	err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

	// Assert
	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]battingFigure{
			{runs: 5, balls: 4, strikeRate: 125},
			{runs: 0, balls: 2, strikeRate: 0},
		},
		spy.battingByInnings[1],
		"A1 faced the four, the no-ball, the byes and the penalty ball and not the wide; A2 faced two")
	assert.Equal(t, bowlingFigure{runs: 7, maidens: 0}, spy.bowlingByInnings[1],
		"B1's figures are the bowler's count and do not move")
}
