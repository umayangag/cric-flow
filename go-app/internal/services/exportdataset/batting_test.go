package exportdataset_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/exportdataset"
)

type fakeRepo struct {
	batUnified   [][]string
	batLegacy    [][]string
	batInfer     map[string][][]string
	batFmt       map[string][][]string
	batErr       error
	bowUnified   [][]string
	bowLegacy    [][]string
	bowInfer     map[string][][]string
	bowFmt       map[string][][]string
	bowErr       error
}

func (f *fakeRepo) BattingUnifiedRows(context.Context) ([][]string, error) {
	if f.batErr != nil { return nil, f.batErr }
	return f.batUnified, nil
}
func (f *fakeRepo) BattingLegacyRows(context.Context) ([][]string, error) {
	if f.batErr != nil { return nil, f.batErr }
	return f.batLegacy, nil
}
func (f *fakeRepo) BattingInferenceRows(_ context.Context, format string) ([][]string, error) {
	if f.batErr != nil { return nil, f.batErr }
	return f.batInfer[format], nil
}
func (f *fakeRepo) BattingFormatRows(_ context.Context, format string) ([][]string, error) {
	if f.batErr != nil { return nil, f.batErr }
	return f.batFmt[format], nil
}
func (f *fakeRepo) BowlingUnifiedRows(context.Context) ([][]string, error) { return f.bowUnified, f.bowErr }
func (f *fakeRepo) BowlingLegacyRows(context.Context) ([][]string, error) { return f.bowLegacy, f.bowErr }
func (f *fakeRepo) BowlingInferenceRows(_ context.Context, format string) ([][]string, error) {
	if f.bowErr != nil { return nil, f.bowErr }
	return f.bowInfer[format], nil
}
func (f *fakeRepo) BowlingFormatRows(_ context.Context, format string) ([][]string, error) {
	if f.bowErr != nil { return nil, f.bowErr }
	return f.bowFmt[format], nil
}

type assertFn func(t *testing.T, w *bytes.Buffer, err error)

type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) { return 0, errors.New("sink write error") }

func assertNoErrorCSV(want string) assertFn {
	return func(t *testing.T, w *bytes.Buffer, err error) {
		if err != nil { t.Fatalf("unexpected err: %v", err) }
		got := w.String()
		if got != want { t.Fatalf("csv mismatch\nwant:\n%s\n---\ngot:\n%s", want, got) }
	}
}

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, _ *bytes.Buffer, err error) {
		s := ""
		if err != nil { s = err.Error() }
		if err == nil || indexOf(s, sub) < 0 { t.Fatalf("want err containing %q, got %v", sub, err) }
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ { if s[i+j] != sub[j] { ok = false; break } }
		if ok { return i }
	}
	return -1
}

func TestBattingService_Exports(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{
		batUnified: [][]string{{"h1","h2"},{"a","b"}},
		batLegacy:  [][]string{{"lh1","lh2"},{"x","y"}},
		batInfer:   map[string][][]string{"ODI": {{"ih1","ih2"},{"m","n"}}},
	}
	service := svc.NewBattingService(repo)

	cases := []struct{
		name string
		act  func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer) error
		want string
		assert assertFn
	}{
		{
			name: "unified writes rows",
			act: func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer) error { return s.ExportUnified(ctx, w) },
			want: "h1,h2\na,b\n",
			assert: assertNoErrorCSV("h1,h2\na,b\n"),
		},
		{
			name: "legacy writes rows",
			act: func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer) error { return s.ExportLegacy(ctx, w) },
			want: "lh1,lh2\nx,y\n",
			assert: assertNoErrorCSV("lh1,lh2\nx,y\n"),
		},
		{
			name: "inference writes rows",
			act: func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer) error { return s.ExportInference(ctx, "ODI", w) },
			want: "ih1,ih2\nm,n\n",
			assert: assertNoErrorCSV("ih1,ih2\nm,n\n"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := tc.act(context.Background(), service, buf)
			_ = tc.want // want is duplicated in assert helper for no-if rule
			tc.assert(t, buf, err)
		})
	}
}

func TestBattingService_Errors(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{batErr: errors.New("boom")}
	s := svc.NewBattingService(repo)
	cases := []struct{
		name string
		act  func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer) error
		assert assertFn
	}{
		{"unified error", func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer) error { return s.ExportUnified(ctx, w) }, assertErrContains("boom")},
		{"legacy error", func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer) error { return s.ExportLegacy(ctx, w) }, assertErrContains("boom")},
		{"inference error", func(ctx context.Context, s *svc.BattingService, w *bytes.Buffer) error { return s.ExportInference(ctx, "ODI", w) }, assertErrContains("boom")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			err := tc.act(context.Background(), s, buf)
			tc.assert(t, buf, err)
		})
	}
}


func TestBattingService_WriterError(t *testing.T) {
	// table-driven style with single case focusing on writer error path
	t.Parallel()
	repo := &fakeRepo{batUnified: [][]string{{"h1"},{"v"}}}
	s := svc.NewBattingService(repo)
	cases := []struct{
		name string
		act  func(ctx context.Context, s *svc.BattingService, w io.Writer) error
		assert assertFn
	}{
		{"write fails", func(ctx context.Context, s *svc.BattingService, w io.Writer) error { return s.ExportUnified(ctx, w) }, assertErrContains("sink write error")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.act(context.Background(), s, errWriter{})
			// pass nil buffer to assert since it does not use it in error path
			tc.assert(t, nil, err)
		})
	}
}
