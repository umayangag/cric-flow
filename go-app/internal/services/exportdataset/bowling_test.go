package exportdataset_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/exportdataset"
)

type fakeRepoB struct {
	batUnified [][]string
	batLegacy  [][]string
	batInfer   map[string][][]string
	batFmt     map[string][][]string
	batErr     error
	bowUnified [][]string
	bowLegacy  [][]string
	bowInfer   map[string][][]string
	bowFmt     map[string][][]string
	bowErr     error
}

func (f *fakeRepoB) BattingUnifiedRows(context.Context) ([][]string, error) {
	return f.batUnified, f.batErr
}

func (f *fakeRepoB) BattingLegacyRows(context.Context) ([][]string, error) {
	return f.batLegacy, f.batErr
}

func (f *fakeRepoB) BattingInferenceRows(_ context.Context, format string) ([][]string, error) {
	if f.batErr != nil {
		return nil, f.batErr
	}
	return f.batInfer[format], nil
}

func (f *fakeRepoB) BattingFormatRows(_ context.Context, format string) ([][]string, error) {
	if f.batErr != nil {
		return nil, f.batErr
	}
	return f.batFmt[format], nil
}

func (f *fakeRepoB) BowlingUnifiedRows(context.Context) ([][]string, error) {
	if f.bowErr != nil {
		return nil, f.bowErr
	}
	return f.bowUnified, nil
}

func (f *fakeRepoB) BowlingLegacyRows(context.Context) ([][]string, error) {
	if f.bowErr != nil {
		return nil, f.bowErr
	}
	return f.bowLegacy, nil
}

func (f *fakeRepoB) BowlingInferenceRows(_ context.Context, format string) ([][]string, error) {
	if f.bowErr != nil {
		return nil, f.bowErr
	}
	return f.bowInfer[format], nil
}

func (f *fakeRepoB) BowlingFormatRows(_ context.Context, format string) ([][]string, error) {
	if f.bowErr != nil {
		return nil, f.bowErr
	}
	return f.bowFmt[format], nil
}

type assertFnB func(t *testing.T, w *bytes.Buffer, err error)

type errWriterB struct{}

func (errWriterB) Write(_ []byte) (int, error) { return 0, errors.New("sink write error") }

func assertNoErrorCSVB(want string) assertFnB {
	return func(t *testing.T, w *bytes.Buffer, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if w.String() != want {
			t.Fatalf("csv mismatch\nwant:\n%s\n---\ngot:\n%s", want, w.String())
		}
	}
}

func assertErrContainsB(sub string) assertFnB {
	return func(t *testing.T, _ *bytes.Buffer, err error) {
		s := ""
		if err != nil {
			s = err.Error()
		}
		if err == nil || indexOfB(s, sub) < 0 {
			t.Fatalf("want err containing %q, got %v", sub, err)
		}
	}
}

func indexOfB(s, sub string) int {
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

func TestBowlingService_Exports(t *testing.T) {
	t.Parallel()
	repo := &fakeRepoB{
		bowUnified: [][]string{{"h1", "h2"}, {"1", "2"}},
		bowLegacy:  [][]string{{"lh1", "lh2"}, {"3", "4"}},
		bowInfer:   map[string][][]string{"T20I": {{"ih1", "ih2"}, {"5", "6"}}},
	}
	s := svc.NewBowlingService(repo)

	cases := []struct {
		name   string
		act    func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer) error
		want   string
		assert assertFnB
	}{
		{"unified writes rows", func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer) error {
			return s.ExportUnified(ctx, w)
		}, "h1,h2\n1,2\n", assertNoErrorCSVB("h1,h2\n1,2\n")},
		{
			"legacy writes rows",
			func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer) error { return s.ExportLegacy(ctx, w) },
			"lh1,lh2\n3,4\n",
			assertNoErrorCSVB("lh1,lh2\n3,4\n"),
		},
		{"inference writes rows", func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer) error {
			return s.ExportInference(ctx, "T20I", w)
		}, "ih1,ih2\n5,6\n", assertNoErrorCSVB("ih1,ih2\n5,6\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := tc.act(context.Background(), s, buf)
			tc.assert(t, buf, err)
		})
	}
}

func TestBowlingService_Errors(t *testing.T) {
	t.Parallel()
	repo := &fakeRepoB{bowErr: errors.New("fail")}
	s := svc.NewBowlingService(repo)
	cases := []struct {
		name   string
		act    func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer) error
		assert assertFnB
	}{
		{"unified error", func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer) error {
			return s.ExportUnified(ctx, w)
		}, assertErrContainsB("fail")},
		{
			"legacy error",
			func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer) error { return s.ExportLegacy(ctx, w) },
			assertErrContainsB("fail"),
		},
		{"inference error", func(ctx context.Context, s *svc.BowlingService, w *bytes.Buffer) error {
			return s.ExportInference(ctx, "T20I", w)
		}, assertErrContainsB("fail")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := tc.act(context.Background(), s, buf)
			tc.assert(t, buf, err)
		})
	}
}

func TestBowlingService_WriterError(t *testing.T) {
	t.Parallel()
	repo := &fakeRepoB{bowUnified: [][]string{{"h1"}, {"v"}}}
	s := svc.NewBowlingService(repo)
	cases := []struct {
		name   string
		act    func(ctx context.Context, s *svc.BowlingService, w io.Writer) error
		assert assertFnB
	}{
		{
			"write fails",
			func(ctx context.Context, s *svc.BowlingService, w io.Writer) error { return s.ExportUnified(ctx, w) },
			assertErrContainsB("sink write error"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.act(context.Background(), s, errWriterB{})
			tc.assert(t, nil, err)
		})
	}
}
