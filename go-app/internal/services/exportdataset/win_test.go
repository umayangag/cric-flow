package exportdataset_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dbmocks "github.com/umayangag/cric-flow/go-app/internal/db/mocks"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"
)

func TestWinService_Exports(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		act    func(ctx context.Context, s *svc.WinService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error
		want   string
		assert assertFn
	}{
		{
			name: "unified writes rows",
			act: func(ctx context.Context, s *svc.WinService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
				m.EXPECT().WinUnifiedRows(mock.Anything).Return([][]string{{"h1", "h2"}, {"a", "b"}}, nil)
				return s.ExportUnified(ctx, w)
			},
			want:   "h1,h2\na,b\n",
			assert: assertNoErrorCSV("h1,h2\na,b\n"),
		},
		{
			name: "format writes rows",
			act: func(ctx context.Context, s *svc.WinService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
				m.EXPECT().WinFormatRows(mock.Anything, "T20I").Return([][]string{{"fh1", "fh2"}, {"x", "y"}}, nil)
				return s.ExportFormat(ctx, "T20I", w)
			},
			want:   "fh1,fh2\nx,y\n",
			assert: assertNoErrorCSV("fh1,fh2\nx,y\n"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := dbmocks.NewMockDatasetRepo(t)
			service := svc.NewWinService(m)
			buf := &bytes.Buffer{}
			err := tc.act(context.Background(), service, buf, m)
			_ = tc.want
			tc.assert(t, buf, err)
		})
	}
}

func TestWinService_Errors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		act    func(ctx context.Context, s *svc.WinService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error
		assert assertFn
	}{
		{
			"unified error",
			func(ctx context.Context, s *svc.WinService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
				m.EXPECT().WinUnifiedRows(mock.Anything).Return(nil, errors.New("boom"))
				return s.ExportUnified(ctx, w)
			},
			assertErrContains("boom"),
		},
		{
			"format error",
			func(ctx context.Context, s *svc.WinService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
				m.EXPECT().WinFormatRows(mock.Anything, "TEST").Return(nil, errors.New("boom"))
				return s.ExportFormat(ctx, "TEST", w)
			},
			assertErrContains("boom"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := dbmocks.NewMockDatasetRepo(t)
			s := svc.NewWinService(m)
			buf := &bytes.Buffer{}
			err := tc.act(context.Background(), s, buf, m)
			tc.assert(t, buf, err)
		})
	}
}

func TestWinService_WriterError(t *testing.T) {
	t.Parallel()
	m := dbmocks.NewMockDatasetRepo(t)
	s := svc.NewWinService(m)
	m.EXPECT().WinUnifiedRows(mock.Anything).Return([][]string{{"h1"}, {"v"}}, nil)
	err := s.ExportUnified(context.Background(), errWriter{})
	require.Error(t, err)
	require.ErrorContains(t, err, "sink write error")
}
