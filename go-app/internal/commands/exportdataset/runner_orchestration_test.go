package exportdataset_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/exportdataset"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/exportdataset"
)

type memFS struct {
	mkdirPath string
	mkdirPerm fs.FileMode
	writes    map[string][]byte
	writePerm map[string]fs.FileMode
	mkdirErr  error
	writeErr  error
}

func (m *memFS) ReadFile(_ context.Context, path string) ([]byte, error) { return m.writes[path], nil }
func (m *memFS) WriteFile(_ context.Context, path string, data []byte, perm fs.FileMode) error {
	if m.writes == nil {
		m.writes = map[string][]byte{}
	}
	if m.writePerm == nil {
		m.writePerm = map[string]fs.FileMode{}
	}
	if m.writeErr != nil {
		return m.writeErr
	}
	m.writes[path] = append([]byte(nil), data...)
	m.writePerm[path] = perm
	return nil
}

func (m *memFS) MkdirAll(path string, perm fs.FileMode) error {
	m.mkdirPath, m.mkdirPerm = path, perm
	return m.mkdirErr
}
func (m *memFS) Glob(_ string) ([]string, error) { return nil, errors.New("not implemented") }

type fakeBat struct{ unified, legacy, infer, format int }

type fakeBow struct{ unified, legacy, infer, format int }

func (f *fakeBat) ExportUnified(_ context.Context, w io.Writer) error {
	f.unified++
	_, _ = w.Write([]byte("buh1,buh2\nA,B\n"))
	return nil
}

func (f *fakeBat) ExportLegacy(_ context.Context, w io.Writer) error {
	f.legacy++
	_, _ = w.Write([]byte("blh1,blh2\nX,Y\n"))
	return nil
}

func (f *fakeBat) ExportInference(_ context.Context, format string, w io.Writer) error {
	f.infer++
	_, _ = w.Write([]byte("bih1,bih2\nI,J\n"))
	return nil
}

func (f *fakeBat) ExportFormat(_ context.Context, format string, w io.Writer) error {
	f.format++
	_, _ = w.Write([]byte("bfh1,bfh2\nQ,R\n"))
	return nil
}

func (f *fakeBow) ExportUnified(_ context.Context, w io.Writer) error {
	f.unified++
	_, _ = w.Write([]byte("woh1,woh2\n1,2\n"))
	return nil
}

func (f *fakeBow) ExportLegacy(_ context.Context, w io.Writer) error {
	f.legacy++
	_, _ = w.Write([]byte("wlh1,wlh2\n3,4\n"))
	return nil
}

func (f *fakeBow) ExportInference(_ context.Context, format string, w io.Writer) error {
	f.infer++
	_, _ = w.Write([]byte("wih1,wih2\n5,6\n"))
	return nil
}

func (f *fakeBow) ExportFormat(_ context.Context, format string, w io.Writer) error {
	f.format++
	_, _ = w.Write([]byte("wfh1,wfh2\n7,8\n"))
	return nil
}

type assertOrchFn func(t *testing.T, fs *memFS, bat *fakeBat, bow *fakeBow, err error)

func assertNoErrorUnifiedOrch(outDir string) assertOrchFn {
	return func(t *testing.T, fs *memFS, bat *fakeBat, bow *fakeBow, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if bat.unified != 1 || bow.unified != 1 {
			t.Fatalf("want unified calls bat=1 bow=1, got %d %d", bat.unified, bow.unified)
		}
		bpath := filepath.Join(outDir, "batting_encoded_all.csv")
		wpath := filepath.Join(outDir, "bowling_encoded_all.csv")
		if string(fs.writes[bpath]) != "buh1,buh2\nA,B\n" {
			t.Fatalf("unexpected batting data: %q", string(fs.writes[bpath]))
		}
		if string(fs.writes[wpath]) != "woh1,woh2\n1,2\n" {
			t.Fatalf("unexpected bowling data: %q", string(fs.writes[wpath]))
		}
	}
}

func assertNoErrorLegacyOrch(outDir string) assertOrchFn {
	return func(t *testing.T, fs *memFS, bat *fakeBat, bow *fakeBow, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if bat.legacy != 1 || bow.legacy != 1 {
			t.Fatalf("want legacy calls bat=1 bow=1, got %d %d", bat.legacy, bow.legacy)
		}
		bpath := filepath.Join(outDir, "batting_encoded.csv")
		wpath := filepath.Join(outDir, "bowling_encoded.csv")
		if string(fs.writes[bpath]) != "blh1,blh2\nX,Y\n" {
			t.Fatalf("unexpected batting data: %q", string(fs.writes[bpath]))
		}
		if string(fs.writes[wpath]) != "wlh1,wlh2\n3,4\n" {
			t.Fatalf("unexpected bowling data: %q", string(fs.writes[wpath]))
		}
	}
}

func assertNoErrorInferOrch(outDir string, fmtcode string) assertOrchFn {
	return func(t *testing.T, fs *memFS, bat *fakeBat, bow *fakeBow, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if bat.infer != 1 || bow.infer != 1 {
			t.Fatalf("want infer calls bat=1 bow=1, got %d %d", bat.infer, bow.infer)
		}
		bpath := filepath.Join(outDir, "batting_infer_"+fmtcode+".csv")
		wpath := filepath.Join(outDir, "bowling_infer_"+fmtcode+".csv")
		if string(fs.writes[bpath]) != "bih1,bih2\nI,J\n" {
			t.Fatalf("unexpected batting data: %q", string(fs.writes[bpath]))
		}
		if string(fs.writes[wpath]) != "wih1,wih2\n5,6\n" {
			t.Fatalf("unexpected bowling data: %q", string(fs.writes[wpath]))
		}
	}
}

func assertNoErrorFormatOrch(outDir string, fmtcode string) assertOrchFn {
	return func(t *testing.T, fs *memFS, bat *fakeBat, bow *fakeBow, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if bat.format != 1 || bow.format != 1 {
			t.Fatalf("want format calls bat=1 bow=1, got %d %d", bat.format, bow.format)
		}
		bpath := filepath.Join(outDir, "batting_encoded_"+fmtcode+".csv")
		wpath := filepath.Join(outDir, "bowling_encoded_"+fmtcode+".csv")
		if string(fs.writes[bpath]) != "bfh1,bfh2\nQ,R\n" {
			t.Fatalf("unexpected batting data: %q", string(fs.writes[bpath]))
		}
		if string(fs.writes[wpath]) != "wfh1,wfh2\n7,8\n" {
			t.Fatalf("unexpected bowling data: %q", string(fs.writes[wpath]))
		}
	}
}

func TestRunner_Orchestrates_Unified(t *testing.T) {
	t.Parallel()
	fsys := &memFS{}
	bat := &fakeBat{}
	bow := &fakeBow{}
	r := cmd.NewRunnerWithServices(fsys, bat, bow)
	opts := cli.Options{OutDir: t.TempDir(), Unified: true}
	err := r.Run(context.Background(), opts)
	assertNoErrorUnifiedOrch(opts.OutDir)(t, fsys, bat, bow, err)
}

func TestRunner_Orchestrates_LegacyCombined(t *testing.T) {
	t.Parallel()
	fsys := &memFS{}
	bat := &fakeBat{}
	bow := &fakeBow{}
	r := cmd.NewRunnerWithServices(fsys, bat, bow)
	opts := cli.Options{OutDir: t.TempDir(), Formats: []string{""}}
	err := r.Run(context.Background(), opts)
	assertNoErrorLegacyOrch(opts.OutDir)(t, fsys, bat, bow, err)
}

func TestRunner_Orchestrates_InferenceOnly(t *testing.T) {
	to := t
	to.Parallel()
	fsys := &memFS{}
	bat := &fakeBat{}
	bow := &fakeBow{}
	r := cmd.NewRunnerWithServices(fsys, bat, bow)
	opts := cli.Options{OutDir: to.TempDir(), InferenceOnly: true, Formats: []string{"ODI"}}
	err := r.Run(context.Background(), opts)
	assertNoErrorInferOrch(opts.OutDir, "ODI")(to, fsys, bat, bow, err)
}

func TestRunner_WriteFileError_Propagates(t *testing.T) {
	t.Parallel()
	fsys := &memFS{writeErr: errors.New("disk full")}
	bat := &fakeBat{}
	bow := &fakeBow{}
	r := cmd.NewRunnerWithServices(fsys, bat, bow)
	// unified path triggers two writes; the first should fail and propagate
	opts := cli.Options{OutDir: t.TempDir(), Unified: true}
	err := r.Run(context.Background(), opts)
	if err == nil || (err != nil && !containsErr(err.Error(), "disk full")) {
		t.Fatalf("expected write error to propagate, got %v", err)
	}
}

// containsErr is a tiny helper to avoid importing strings for a single use.
func containsErr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
