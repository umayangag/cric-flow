package teamselect_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"
)

func TestSelect_CannotReplaceToSatisfyBowlers(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	pool := []ts.Player{
		{Name: "B1", BowlScore: 0.9, IsBowler: true},
		{Name: "B2", BowlScore: 0.8, IsBowler: true},
		{Name: "B3", BowlScore: 0.7, IsBowler: true},
	}
	_, err := ts.Select(pool, w, ts.Constraints{Size: 2, MinBowlers: 3})
	require.Error(t, err, "want error when cannot replace to satisfy bowler constraint")
}

func TestLoadFromCSV_EmptyFloatsAndBoolVariants(t *testing.T) {
	t.Parallel()
	csv := "name,is_bowler,is_keeper,bat_score,bowl_score\nA,yes,no,,\nB,true,1, ,0.4\nC,0,YES,0.3, \n"
	ps, err := ts.LoadFromCSV(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, ps, 3)
	require.Equal(t, 0.0, ps[0].BatScore)
	require.Equal(t, 0.0, ps[0].BowlScore)
	require.True(t, ps[0].IsBowler)
	require.False(t, ps[0].IsKeeper)
	require.Equal(t, 0.0, ps[1].BatScore)
	require.Equal(t, 0.4, ps[1].BowlScore)
	require.True(t, ps[1].IsBowler)
	require.True(t, ps[1].IsKeeper)
	require.True(t, ps[2].IsKeeper)
}
