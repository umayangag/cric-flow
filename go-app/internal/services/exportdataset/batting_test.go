package exportdataset_test

import (
    "bytes"
    "context"
    "errors"
    "io"
    "testing"

    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
    dbmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/db/mocks"
    svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/exportdataset"
)

type assertFn func(t *testing.T, w *bytes.Buffer, err error)

type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) { return 0, errors.New("sink write error") }

func assertNoErrorCSV(want string) assertFn {
    return func(t *testing.T, w *bytes.Buffer, err error) {
        require.NoError(t, err)
        require.Equal(t, want, w.String())
    }
}

func assertErrContains(sub string) assertFn {
    return func(t *testing.T, _ *bytes.Buffer, err error) {
        require.Error(t, err)
        require.ErrorContains(t, err, sub)
    }
}

func TestBattingService_Exports(t *testing.T) {
    t.Parallel()

    cases := []struct {
        name   string
        act    func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error
        want   string
        assert assertFn
    }{
        {
            name: "unified writes rows",
            act: func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
                m.EXPECT().BattingUnifiedRows(mock.Anything).Return([][]string{{"h1", "h2"}, {"a", "b"}}, nil)
                return s.ExportUnified(ctx, w)
            },
            want:   "h1,h2\na,b\n",
            assert: assertNoErrorCSV("h1,h2\na,b\n"),
        },
        {
            name:   "legacy writes rows",
            act:    func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
                m.EXPECT().BattingLegacyRows(mock.Anything).Return([][]string{{"lh1", "lh2"}, {"x", "y"}}, nil)
                return s.ExportLegacy(ctx, w)
            },
            want:   "lh1,lh2\nx,y\n",
            assert: assertNoErrorCSV("lh1,lh2\nx,y\n"),
        },
        {
            name: "inference writes rows",
            act: func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
                m.EXPECT().BattingInferenceRows(mock.Anything, "ODI").Return([][]string{{"ih1", "ih2"}, {"m", "n"}}, nil)
                return s.ExportInference(ctx, "ODI", w)
            },
            want:   "ih1,ih2\nm,n\n",
            assert: assertNoErrorCSV("ih1,ih2\nm,n\n"),
        },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            m := dbmocks.NewMockDatasetRepo(t)
            service := svc.NewBattingService(m)
            buf := &bytes.Buffer{}
            err := tc.act(context.Background(), service, buf, m)
            _ = tc.want // want is duplicated in assert helper for no-if rule
            tc.assert(t, buf, err)
        })
    }
}

func TestBattingService_Errors(t *testing.T) {
    t.Parallel()
    cases := []struct {
        name   string
        act    func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error
        assert assertFn
    }{
        {"unified error", func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
            m.EXPECT().BattingUnifiedRows(mock.Anything).Return(nil, errors.New("boom"))
            return s.ExportUnified(ctx, w)
        }, assertErrContains("boom")},
        {
            "legacy error",
            func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
                m.EXPECT().BattingLegacyRows(mock.Anything).Return(nil, errors.New("boom"))
                return s.ExportLegacy(ctx, w)
            },
            assertErrContains("boom"),
        },
        {"inference error", func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer, m *dbmocks.MockDatasetRepo) error {
            m.EXPECT().BattingInferenceRows(mock.Anything, "ODI").Return(nil, errors.New("boom"))
            return s.ExportInference(ctx, "ODI", w)
        }, assertErrContains("boom")},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            m := dbmocks.NewMockDatasetRepo(t)
            s := svc.NewBattingService(m)
            buf := &bytes.Buffer{}
            err := tc.act(context.Background(), s, buf, m)
            tc.assert(t, buf, err)
        })
    }
}

func TestBattingService_WriterError(t *testing.T) {
    // table-driven style with single case focusing on writer error path
    t.Parallel()
    m := dbmocks.NewMockDatasetRepo(t)
    s := svc.NewBattingService(m)
    cases := []struct {
        name   string
        act    func(ctx context.Context, s *svc.BattingService, w io.Writer) error
        assert assertFn
    }{
        {
            "write fails",
            func(ctx context.Context, s *svc.BattingService, w io.Writer) error {
                m.EXPECT().BattingUnifiedRows(mock.Anything).Return([][]string{{"h1"}, {"v"}}, nil)
                return s.ExportUnified(ctx, w)
            },
            assertErrContains("sink write error"),
        },
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            err := tc.act(context.Background(), s, errWriter{})
            // pass nil buffer to assert since it does not use it in error path
            tc.assert(t, nil, err)
        })
    }
}
