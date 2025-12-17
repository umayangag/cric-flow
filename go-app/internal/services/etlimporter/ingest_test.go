package etlimporter_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/etlimporter"
)

// helper to write files to a temp directory for IngestDir
func writeTempFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
}

type fakeRepo struct {
	bat    int
	bowl   int
	errBat error
	errBwl error
}

func (r *fakeRepo) UpsertBatting(_ context.Context, rows []db.EtlBattingRow) error {
	if r.errBat != nil {
		return r.errBat
	}
	r.bat += len(rows)
	return nil
}

func (r *fakeRepo) UpsertBowling(_ context.Context, rows []db.EtlBowlingRow) error {
	if r.errBwl != nil {
		return r.errBwl
	}
	r.bowl += len(rows)
	return nil
}

func TestService_IngestDir(t *testing.T) {
	t.Parallel()
	batCSV := "player_name,season,format,runs,balls,fours,sixes,position\nA,2019,T20,10,8,1,0,3\n"
	bwlCSV := "player_name,season,format,overs,balls,maidens,runs,wickets,economy\nB,2019,ODI,10,60,0,40,1,4.0\n"
	cases := []struct {
		name   string
		apply  bool
		setup  func(dir string)
		repo   *fakeRepo
		assert func(t *testing.T, st etlimporter.Stats, repo *fakeRepo, err error)
	}{
		{
			name:  "dry-run parses batting and bowling",
			apply: false,
			setup: func(dir string) {
				writeTempFile(t, dir, "a.csv", batCSV)
				writeTempFile(t, dir, "b.csv", bwlCSV)
			},
			repo: &fakeRepo{},
			assert: func(t *testing.T, st etlimporter.Stats, _ *fakeRepo, err error) {
				require.NoError(t, err)
				require.Equal(t, 2, st.Files)
				require.Equal(t, 1, st.BattingRows)
				require.Equal(t, 1, st.BowlingRows)
			},
		},
		{
			name:  "apply upserts successfully",
			apply: true,
			setup: func(dir string) {
				writeTempFile(t, dir, "a.csv", batCSV)
				writeTempFile(t, dir, "b.csv", bwlCSV)
			},
			repo: &fakeRepo{},
			assert: func(t *testing.T, st etlimporter.Stats, repo *fakeRepo, err error) {
				require.NoError(t, err)
				require.Equal(t, 2, st.Files)
				require.Equal(t, 1, st.BattingRows)
				require.Equal(t, 1, st.BowlingRows)
				// Side-effects: repo call counts should match rows when apply=true
				require.Equal(t, 1, repo.bat)
				require.Equal(t, 1, repo.bowl)
			},
		},
		{
			name:  "parse error surfaces",
			apply: false,
			setup: func(dir string) { writeTempFile(t, dir, "bad.csv", "x,y\n1,2\n") },
			repo:  &fakeRepo{},
			assert: func(t *testing.T, _ etlimporter.Stats, _ *fakeRepo, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "unexpected batting header")
			},
		},
		{
			name:  "repo error surfaces",
			apply: true,
			setup: func(dir string) { writeTempFile(t, dir, "a.csv", batCSV) },
			repo:  &fakeRepo{errBat: errors.New("boom")},
			assert: func(t *testing.T, _ etlimporter.Stats, _ *fakeRepo, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "boom")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := etlimporter.NewService(tc.repo)
			dir := t.TempDir()
			if tc.setup != nil {
				tc.setup(dir)
			}
			st, err := svc.IngestDir(context.Background(), dir, "*.csv", tc.apply, 1)
			// NB: we do not assert repo counters explicitly in dry-run since service returns Stats
			tc.assert(t, st, tc.repo, err)
		})
	}
}
