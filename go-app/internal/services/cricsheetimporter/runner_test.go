package cricsheetimporter_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/cricsheetimporter"
	cricsheetmocks "github.com/umayangag/cric-flow/go-app/internal/cricsheet/mocks"
	dbmocks "github.com/umayangag/cric-flow/go-app/internal/db/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/models"
)

func TestRunner_Run_Table(t *testing.T) {
	t.Parallel()

	type arrangeFn func(l *cricsheetmocks.MockLoader, p *cricsheetmocks.MockParser, r *dbmocks.MockMatchRepo)
	type assertFn func(t *testing.T, err error)

	cases := []struct {
		name    string
		opts    svc.Options
		arrange arrangeFn
		assert  assertFn
		custom  func(t *testing.T) error
	}{
		{
			name: "nil runner",
			opts: svc.Options{InDir: "/tmp"},
			custom: func(_ *testing.T) error {
				var rnil *svc.Runner
				return rnil.Run(context.Background(), svc.Options{InDir: "/tmp"})
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "nil runner")
			},
		},
		{
			name: "missing dependency loader",
			opts: svc.Options{InDir: "/tmp"},
			custom: func(t *testing.T) error {
				p := cricsheetmocks.NewMockParser(t)
				r := dbmocks.NewMockMatchRepo(t)
				runner := svc.NewRunner(nil, p, r, nil)
				return runner.Run(context.Background(), svc.Options{InDir: "/tmp"})
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "missing dependency")
			},
		},
		{
			name: "missing dependency parser",
			opts: svc.Options{InDir: "/tmp"},
			custom: func(t *testing.T) error {
				l := cricsheetmocks.NewMockLoader(t)
				r := dbmocks.NewMockMatchRepo(t)
				runner := svc.NewRunner(l, nil, r, nil)
				return runner.Run(context.Background(), svc.Options{InDir: "/tmp"})
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "missing dependency")
			},
		},
		{
			name: "missing dependency repository",
			opts: svc.Options{InDir: "/tmp"},
			custom: func(t *testing.T) error {
				l := cricsheetmocks.NewMockLoader(t)
				p := cricsheetmocks.NewMockParser(t)
				runner := svc.NewRunner(l, p, nil, nil)
				return runner.Run(context.Background(), svc.Options{InDir: "/tmp"})
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "missing dependency")
			},
		},
		{
			name: "empty InDir",
			opts: svc.Options{InDir: ""},
			arrange: func(_ *cricsheetmocks.MockLoader, _ *cricsheetmocks.MockParser, _ *dbmocks.MockMatchRepo) {
				// No expectations - should fail before calling deps
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "input directory is required")
			},
		},
		{
			name: "List returns error",
			opts: svc.Options{InDir: "/data"},
			arrange: func(l *cricsheetmocks.MockLoader, _ *cricsheetmocks.MockParser, _ *dbmocks.MockMatchRepo) {
				l.EXPECT().List(mock.Anything, "/data").Return(nil, errors.New("list failed"))
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "list inputs")
			},
		},
		{
			name: "Load returns error",
			opts: svc.Options{InDir: "/data"},
			arrange: func(l *cricsheetmocks.MockLoader, _ *cricsheetmocks.MockParser, _ *dbmocks.MockMatchRepo) {
				l.EXPECT().List(mock.Anything, "/data").Return([]string{"a.json"}, nil)
				l.EXPECT().Load(mock.Anything, "/data", "a.json").Return(nil, errors.New("load failed"))
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "load a.json")
			},
		},
		{
			name: "Parse returns error",
			opts: svc.Options{InDir: "/data"},
			arrange: func(l *cricsheetmocks.MockLoader, p *cricsheetmocks.MockParser, _ *dbmocks.MockMatchRepo) {
				l.EXPECT().List(mock.Anything, "/data").Return([]string{"a.json"}, nil)
				l.EXPECT().Load(mock.Anything, "/data", "a.json").Return([]byte("{}"), nil)
				p.EXPECT().Parse(mock.Anything, []byte("{}")).Return(nil, errors.New("parse failed"))
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "parse a.json")
			},
		},
		{
			name: "dry-run happy path",
			opts: svc.Options{InDir: "/data", Apply: false},
			arrange: func(l *cricsheetmocks.MockLoader, p *cricsheetmocks.MockParser, _ *dbmocks.MockMatchRepo) {
				l.EXPECT().List(mock.Anything, "/data").Return([]string{"m1.json"}, nil)
				l.EXPECT().Load(mock.Anything, "/data", "m1.json").Return([]byte("raw"), nil)
				p.EXPECT().Parse(mock.Anything, []byte("raw")).Return([]models.Match{{ID: 1}}, nil)
				// No UpsertMatches when Apply=false
			},
			assert: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "apply happy path",
			opts: svc.Options{InDir: "/data", Apply: true},
			arrange: func(l *cricsheetmocks.MockLoader, p *cricsheetmocks.MockParser, r *dbmocks.MockMatchRepo) {
				l.EXPECT().List(mock.Anything, "/data").Return([]string{"m1.json"}, nil)
				l.EXPECT().Load(mock.Anything, "/data", "m1.json").Return([]byte("raw"), nil)
				p.EXPECT().Parse(mock.Anything, []byte("raw")).Return([]models.Match{{ID: 1}, {ID: 2}}, nil)
				r.EXPECT().UpsertMatches(mock.Anything, []models.Match{{ID: 1}, {ID: 2}}).Return(nil)
			},
			assert: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "UpsertMatches returns error",
			opts: svc.Options{InDir: "/data", Apply: true},
			arrange: func(l *cricsheetmocks.MockLoader, p *cricsheetmocks.MockParser, r *dbmocks.MockMatchRepo) {
				l.EXPECT().List(mock.Anything, "/data").Return([]string{"m1.json"}, nil)
				l.EXPECT().Load(mock.Anything, "/data", "m1.json").Return([]byte("raw"), nil)
				p.EXPECT().Parse(mock.Anything, []byte("raw")).Return([]models.Match{{ID: 1}}, nil)
				r.EXPECT().UpsertMatches(mock.Anything, mock.Anything).Return(errors.New("upsert failed"))
			},
			assert: func(t *testing.T, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "upsert matches")
			},
		},
		{
			name: "empty file list dry-run",
			opts: svc.Options{InDir: "/data", Apply: false},
			arrange: func(l *cricsheetmocks.MockLoader, _ *cricsheetmocks.MockParser, _ *dbmocks.MockMatchRepo) {
				l.EXPECT().List(mock.Anything, "/data").Return([]string{}, nil)
			},
			assert: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "parse returns empty matches skips batch",
			opts: svc.Options{InDir: "/data", Apply: false},
			arrange: func(l *cricsheetmocks.MockLoader, p *cricsheetmocks.MockParser, _ *dbmocks.MockMatchRepo) {
				l.EXPECT().List(mock.Anything, "/data").Return([]string{"empty.json"}, nil)
				l.EXPECT().Load(mock.Anything, "/data", "empty.json").Return([]byte("{}"), nil)
				p.EXPECT().Parse(mock.Anything, []byte("{}")).Return([]models.Match{}, nil)
			},
			assert: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var err error
			if tc.custom != nil {
				err = tc.custom(t)
			} else {
				l := cricsheetmocks.NewMockLoader(t)
				p := cricsheetmocks.NewMockParser(t)
				r := dbmocks.NewMockMatchRepo(t)
				if tc.arrange != nil {
					tc.arrange(l, p, r)
				}
				runner := svc.NewRunner(l, p, r, nil)
				err = runner.Run(context.Background(), tc.opts)
			}
			tc.assert(t, err)
		})
	}
}
