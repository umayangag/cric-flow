package exportdataset_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dbmocks "github.com/umayangag/cric-flow/go-app/internal/db/mocks"
	svc "github.com/umayangag/cric-flow/go-app/internal/services/exportdataset"
)

type assertFnB func(t *testing.T, w *bytes.Buffer, err error)

type errWriterB struct{}

func (errWriterB) Write(_ []byte) (int, error) { return 0, errors.New("sink write error") }

func assertNoErrorCSVB(want string) assertFnB {
	return func(t *testing.T, w *bytes.Buffer, err error) {
		require.NoError(t, err)
		require.Equal(t, want, w.String())
	}
}

func assertErrContainsB(sub string) assertFnB {
	return func(t *testing.T, _ *bytes.Buffer, err error) {
		require.Error(t, err)
		require.ErrorContains(t, err, sub)
	}
}

func TestBowlingService_Exports(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		act    func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error
		want   string
		assert assertFnB
	}{
		{
			"unified writes rows",
			func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
				m.EXPECT().BowlingUnifiedRows(mock.Anything).Return([][]string{{"h1", "h2"}, {"1", "2"}}, nil)
				return s.ExportUnified(ctx, w)
			},
			"h1,h2\n1,2\n",
			assertNoErrorCSVB("h1,h2\n1,2\n"),
		},
		{
			"legacy writes rows",
			func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
				m.EXPECT().BowlingLegacyRows(mock.Anything).Return([][]string{{"lh1", "lh2"}, {"3", "4"}}, nil)
				return s.ExportLegacy(ctx, w)
			},
			"lh1,lh2\n3,4\n",
			assertNoErrorCSVB("lh1,lh2\n3,4\n"),
		},
		{
			"inference writes rows",
			func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
				m.EXPECT().
					BowlingInferenceRows(mock.Anything, "T20I").
					Return([][]string{{"ih1", "ih2"}, {"5", "6"}}, nil)
				return s.ExportInference(ctx, "T20I", w)
			},
			"ih1,ih2\n5,6\n",
			assertNoErrorCSVB("ih1,ih2\n5,6\n"),
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			m := dbmocks.NewMockDatasetRepo(t)
			s := svc.NewBowlingService(m)
			buf := &bytes.Buffer{}
			err := tc.act(context.Background(), s, buf, m)
			tc.assert(t, buf, err)
		})
	}
}

func TestBowlingService_Errors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		act    func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error
		assert assertFnB
	}{
		{
			"unified error",
			func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
				m.EXPECT().BowlingUnifiedRows(mock.Anything).Return(nil, errors.New("fail"))
				return s.ExportUnified(ctx, w)
			},
			assertErrContainsB("fail"),
		},
		{
			"legacy error",
			func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
				m.EXPECT().BowlingLegacyRows(mock.Anything).Return(nil, errors.New("fail"))
				return s.ExportLegacy(ctx, w)
			},
			assertErrContainsB("fail"),
		},
		{
			"inference error",
			func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
				m.EXPECT().BowlingInferenceRows(mock.Anything, "T20I").Return(nil, errors.New("fail"))
				return s.ExportInference(ctx, "T20I", w)
			},
			assertErrContainsB("fail"),
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			m := dbmocks.NewMockDatasetRepo(t)
			s := svc.NewBowlingService(m)
			buf := &bytes.Buffer{}
			err := tc.act(context.Background(), s, buf, m)
			tc.assert(t, buf, err)
		})
	}
}

func TestBowlingService_WriterError(t *testing.T) {
	t.Parallel()
	m := dbmocks.NewMockDatasetRepo(t)
	s := svc.NewBowlingService(m)
	testCases := []struct {
		name   string
		act    func(ctx context.Context, s *svc.BowlingService, w io.Writer) error
		assert assertFnB
	}{
		{
			"write fails",
			func(ctx context.Context, s *svc.BowlingService, w io.Writer) error {
				m.EXPECT().BowlingUnifiedRows(mock.Anything).Return([][]string{{"h1"}, {"v"}}, nil)
				return s.ExportUnified(ctx, w)
			},
			assertErrContainsB("sink write error"),
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			err := tc.act(context.Background(), s, errWriterB{})
			tc.assert(t, nil, err)
		})
	}
}
