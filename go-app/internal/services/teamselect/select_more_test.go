package teamselect_test

import (
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
