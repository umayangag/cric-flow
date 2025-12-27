package cricsheetimporter_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dbmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/db/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/cricsheetimporter"
	svcmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/services/cricsheetimporter/internal/mocks"
)

type assertSvcFn func(t *testing.T, processed int, err error)

func assertNoErrorProcessed(want int) assertSvcFn {
	return func(t *testing.T, processed int, err error) {
		require.NoError(t, err)
		require.Equal(t, want, processed)
	}
}

func assertErrContains(sub string) assertSvcFn {
	return func(t *testing.T, _ int, err error) {
		require.Error(t, err)
		require.ErrorContains(t, err, sub)
	}
}

func TestIngestService_BasicFlows(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		apply   bool
		conc    int
		arrange func(l *svcmocks.MockLoader, p *svcmocks.MockParser, r *dbmocks.MockMatchRepo)
		assert  assertSvcFn
	}{
		{
			name:  "dry-run single worker",
			apply: false,
			conc:  1,
			arrange: func(l *svcmocks.MockLoader, p *svcmocks.MockParser, r *dbmocks.MockMatchRepo) {
				_ = r
				l.EXPECT().List(mock.Anything, ".").Return([]string{"a.json", "b.json"}, nil)
				l.EXPECT().Load(mock.Anything, ".", "a.json").Return([]byte("A"), nil).Once()
				l.EXPECT().Load(mock.Anything, ".", "b.json").Return([]byte("B"), nil).Once()
				p.EXPECT().Parse(mock.Anything, []byte("A")).Return([]models.Match{{ID: 1}}, nil)
				p.EXPECT().Parse(mock.Anything, []byte("B")).Return([]models.Match{{ID: 2}, {ID: 3}}, nil)
				// No repository calls when apply=false
			},
			assert: assertNoErrorProcessed(2),
		},
		{
			name:  "apply with 2 workers",
			apply: true,
			conc:  2,
			arrange: func(l *svcmocks.MockLoader, p *svcmocks.MockParser, r *dbmocks.MockMatchRepo) {
				l.EXPECT().List(mock.Anything, ".").Return([]string{"a.json", "b.json"}, nil)
				// Order-agnostic loads
				l.EXPECT().Load(mock.Anything, ".", "a.json").Return([]byte("A"), nil).Once()
				l.EXPECT().Load(mock.Anything, ".", "b.json").Return([]byte("B"), nil).Once()
				p.EXPECT().Parse(mock.Anything, []byte("A")).Return([]models.Match{{ID: 1}}, nil)
				p.EXPECT().Parse(mock.Anything, []byte("B")).Return([]models.Match{{ID: 2}, {ID: 3}}, nil)
				r.EXPECT().UpsertMatches(mock.Anything, mock.Anything).Return(nil).Twice()
			},
			assert: assertNoErrorProcessed(2),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := svcmocks.NewMockLoader(t)
			p := svcmocks.NewMockParser(t)
			r := dbmocks.NewMockMatchRepo(t)
			s := &svc.IngestService{Loader: l, Parser: p, Repository: r}
			if tc.arrange != nil {
				tc.arrange(l, p, r)
			}
			processed, err := s.IngestDir(context.Background(), ".", tc.apply, tc.conc)
			tc.assert(t, processed, err)
		})
	}
}

func TestIngestService_Errors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		dir     string
		arrange func(l *svcmocks.MockLoader, p *svcmocks.MockParser, r *dbmocks.MockMatchRepo)
		svcNil  bool
		assert  assertSvcFn
	}{
		{
			name:   "nil deps",
			dir:    ".",
			svcNil: true,
			assert: assertErrContains("nil service"),
		},
		{
			name: "empty dir",
			dir:  "",
			arrange: func(l *svcmocks.MockLoader, p *svcmocks.MockParser, r *dbmocks.MockMatchRepo) {
				// no expectations; validation fails before use
				_ = l
				_ = p
				_ = r
			},
			assert: assertErrContains("input directory"),
		},
		{
			name: "list error",
			dir:  ".",
			arrange: func(l *svcmocks.MockLoader, p *svcmocks.MockParser, r *dbmocks.MockMatchRepo) {
				_ = p
				_ = r
				l.EXPECT().List(mock.Anything, ".").Return(nil, errors.New("boom"))
			},
			assert: assertErrContains("boom"),
		},
		{
			name: "load error",
			dir:  ".",
			arrange: func(l *svcmocks.MockLoader, p *svcmocks.MockParser, r *dbmocks.MockMatchRepo) {
				_ = p
				_ = r
				l.EXPECT().List(mock.Anything, ".").Return([]string{"y.json"}, nil)
				l.EXPECT().Load(mock.Anything, ".", "y.json").Return(nil, errors.New("missing"))
			},
			assert: assertErrContains("missing"),
		},
		{
			name: "parse error",
			dir:  ".",
			arrange: func(l *svcmocks.MockLoader, p *svcmocks.MockParser, r *dbmocks.MockMatchRepo) {
				_ = r
				l.EXPECT().List(mock.Anything, ".").Return([]string{"x.json"}, nil)
				l.EXPECT().Load(mock.Anything, ".", "x.json").Return([]byte("X"), nil)
				p.EXPECT().Parse(mock.Anything, []byte("X")).Return(nil, errors.New("parse"))
			},
			assert: assertErrContains("parse"),
		},
		{
			name: "repo error",
			dir:  ".",
			arrange: func(l *svcmocks.MockLoader, p *svcmocks.MockParser, r *dbmocks.MockMatchRepo) {
				l.EXPECT().List(mock.Anything, ".").Return([]string{"x.json"}, nil)
				l.EXPECT().Load(mock.Anything, ".", "x.json").Return([]byte("X"), nil)
				p.EXPECT().Parse(mock.Anything, []byte("X")).Return([]models.Match{{ID: 9}}, nil)
				r.EXPECT().UpsertMatches(mock.Anything, mock.Anything).Return(errors.New("db"))
			},
			assert: assertErrContains("db"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s *svc.IngestService
			if tc.svcNil {
				s = &svc.IngestService{}
			} else {
				l := svcmocks.NewMockLoader(t)
				p := svcmocks.NewMockParser(t)
				r := dbmocks.NewMockMatchRepo(t)
				if tc.arrange != nil {
					tc.arrange(l, p, r)
				}
				s = &svc.IngestService{Loader: l, Parser: p, Repository: r}
			}
			_, err := s.IngestDir(context.Background(), tc.dir, true, 1)
			tc.assert(t, 0, err)
		})
	}
}
