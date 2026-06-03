package teamselect_test

import (
	"context"
	"strings"
	"testing"

	ts "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"

	"github.com/stretchr/testify/require"
)

func TestLoadFromCSV_MoreErrors(t *testing.T) {
	t.Parallel()
	// nil reader
	_, err := ts.LoadFromCSV(nil)
	require.Error(t, err)
	// missing name
	badName := "name,is_bowler,is_keeper,bat_score,bowl_score\n ,1,0,0.2,0.3\n"
	_, err = ts.LoadFromCSV(strings.NewReader(badName))
	require.Error(t, err)
	require.True(t, contains2(err.Error(), "name"))
	// bad bowl_score
	badBowl := "name,is_bowler,is_keeper,bat_score,bowl_score\nA,0,0,0.2,abc\n"
	_, err = ts.LoadFromCSV(strings.NewReader(badBowl))
	require.Error(t, err)
	require.True(t, contains2(err.Error(), "bowl_score"))
}

func TestLoadFromDB_ErrPropagate(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{err: context.Canceled}
	_, err := ts.LoadFromDB(context.Background(), repo, 1, "T20", "2019")
	require.Error(t, err)
}

func TestSelect_EdgeErrorsAndTies(t *testing.T) {
	t.Parallel()
	w := ts.DefaultWeights()
	mk := func(name string, bat, bowl float64, isBow, isKeep bool) ts.Player {
		return ts.Player{Name: name, BatScore: bat, BowlScore: bowl, IsBowler: isBow, IsKeeper: isKeep}
	}
	pool := []ts.Player{mk("A", 0.5, 0.5, false, false), mk("B", 0.5, 0.5, false, false)}
	// Equal scores should sort by name ascending deterministically
	team, err := ts.Select(pool, w, ts.Constraints{Size: 1})
	require.NoError(t, err)
	require.Len(t, team, 1)
	require.Equal(t, "A", team[0].Name)
	// invalid constraints
	_, err = ts.Select(pool, w, ts.Constraints{Size: 0})
	require.Error(t, err)
	_, err = ts.Select(pool, w, ts.Constraints{Size: 1, MinBowlers: -1})
	require.Error(t, err)
	// require keeper but none available
	_, err = ts.Select(pool, w, ts.Constraints{Size: 1, RequireKeeper: true})
	require.Error(t, err)
}

func contains2(s, sub string) bool { return indexOf2(s, sub) >= 0 }
func indexOf2(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}
