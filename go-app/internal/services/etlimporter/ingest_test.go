package etlimporter_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/services/etlimporter"
)

type memFS struct {
	files map[string]string
}

func (m *memFS) ReadFile(_ context.Context, path string) ([]byte, error) {
	if m.files == nil {
		return nil, errors.New("no files")
	}
	v, ok := m.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return []byte(v), nil
}

func (m *memFS) WriteFile(_ context.Context, _ string, _ []byte, _ fs.FileMode) error {
	return errors.New("not implemented")
}

func (m *memFS) MkdirAll(_ string, _ fs.FileMode) error { return errors.New("not implemented") }

func (m *memFS) Glob(pattern string) ([]string, error) {
	var out []string
	base := strings.TrimSuffix(pattern, "*.csv")
	for p := range m.files {
		if strings.HasPrefix(p, base) && strings.HasSuffix(p, ".csv") {
			out = append(out, p)
		}
	}
	return out, nil
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
		fs     *memFS
		repo   *fakeRepo
		assert func(t *testing.T, st etlimporter.Stats, repo *fakeRepo, err error)
	}{
		{
			name:  "dry-run parses batting and bowling",
			apply: false,
			fs: &memFS{
				files: map[string]string{
					filepath.Join("/data", "a.csv"): batCSV,
					filepath.Join("/data", "b.csv"): bwlCSV,
				},
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
			fs: &memFS{
				files: map[string]string{
					filepath.Join("/data", "a.csv"): batCSV,
					filepath.Join("/data", "b.csv"): bwlCSV,
				},
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
			fs:    &memFS{files: map[string]string{filepath.Join("/data", "bad.csv"): "x,y\n1,2\n"}},
			repo:  &fakeRepo{},
			assert: func(t *testing.T, _ etlimporter.Stats, _ *fakeRepo, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "unexpected batting header")
			},
		},
		{
			name:  "repo error surfaces",
			apply: true,
			fs:    &memFS{files: map[string]string{filepath.Join("/data", "a.csv"): batCSV}},
			repo:  &fakeRepo{errBat: errors.New("boom")},
			assert: func(t *testing.T, _ etlimporter.Stats, _ *fakeRepo, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "boom")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := etlimporter.NewService(tc.fs, tc.repo)
			st, err := svc.IngestDir(context.Background(), "/data", "*.csv", tc.apply, 1)
			// NB: we do not assert repo counters explicitly in dry-run since service returns Stats
			tc.assert(t, st, tc.repo, err)
		})
	}
}
