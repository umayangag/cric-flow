package teamselect_test

import (
	"context"
	"strings"
	"testing"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	ts "github.com/umayangag/cric-flow/go-app/internal/services/teamselect"

	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	out []db.PoolPlayer
	err error
}

func (r *fakeRepo) LoadPool(_ context.Context, _ int64, _, _ string) ([]db.PoolPlayer, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.out, nil
}

type assertCSVFn func(t *testing.T, ps []ts.Player, err error)

type assertDBFn func(t *testing.T, ps []ts.Player, err error)

func assertNoErrorCountCSV(n int) assertCSVFn {
	return func(t *testing.T, ps []ts.Player, err error) {
		require.NoError(t, err)
		require.Equal(t, n, len(ps))
	}
}

func assertErrContainsCSV(sub string) assertCSVFn {
	return func(t *testing.T, _ []ts.Player, err error) {
		require.Error(t, err)
		require.Contains(t, err.Error(), sub)
	}
}

func assertNoErrorCountDB(n int) assertDBFn {
	return func(t *testing.T, ps []ts.Player, err error) {
		require.NoError(t, err)
		require.Equal(t, n, len(ps))
	}
}

func assertErrContainsDB(sub string) assertDBFn {
	return func(t *testing.T, _ []ts.Player, err error) {
		require.Error(t, err)
		require.Contains(t, err.Error(), sub)
	}
}

func TestLoadFromCSV_Table(t *testing.T) {
	t.Parallel()
	good := "name,is_bowler,is_keeper,bat_score,bowl_score\nA,1,0,0.7,0.2\nB,0,1,0.5,0.1\n"
	badHeader := "x,y\n1,2\n"
	badRow := "name,is_bowler,is_keeper,bat_score,bowl_score\nA,yes,no,abc,0\n"
	testCases := []struct {
		name   string
		csv    string
		assert assertCSVFn
	}{
		{"ok", good, assertNoErrorCountCSV(2)},
		{"bad header", badHeader, assertErrContainsCSV("unexpected header")},
		{"bad value", badRow, assertErrContainsCSV("bat_score")},
		{"empty", "", assertErrContainsCSV("empty csv")},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			ps, err := ts.LoadFromCSV(strings.NewReader(tc.csv))
			tc.assert(t, ps, err)
		})
	}
}

func TestLoadFromDB_Table(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{out: []db.PoolPlayer{{Name: "A"}, {Name: "B"}}}
	ps, err := ts.LoadFromDB(context.Background(), repo, 1, "T20", "2019")
	assertNoErrorCountDB(2)(t, ps, err)
	_, err2 := ts.LoadFromDB(context.Background(), nil, 1, "T20", "2019")
	assertErrContainsDB("nil repo")(t, nil, err2)
	_, err3 := ts.LoadFromDB(context.Background(), repo, 0, "T20", "2019")
	assertErrContainsDB("invalid args")(t, nil, err3)
}
