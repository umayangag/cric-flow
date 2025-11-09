package osfs_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	impl "github.com/umayangag/cric-info-scrapers/go-app/internal/adapters/fsx/osfs"
)

type assertFn func(t *testing.T, err error)

type assertPathFn func(t *testing.T, path string, err error)

func assertNoError(t *testing.T, err error) {
	if err != nil { t.Fatalf("unexpected error: %v", err) }
}

func assertErrorContains(substr string) assertFn {
	return func(t *testing.T, err error) {
		if err == nil || !contains(err.Error(), substr) {
			t.Fatalf("want error containing %q, got %v", substr, err)
		}
	}
}

func assertDirExists() assertPathFn {
	return func(t *testing.T, path string, err error) {
		if err != nil { t.Fatalf("unexpected error: %v", err) }
		st, statErr := os.Stat(path)
		if statErr != nil { t.Fatalf("expected dir to exist: %v", statErr) }
		if !st.IsDir() { t.Fatalf("expected %s to be a directory", path) }
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0) }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] { match = false; break }
		}
		if match { return i }
	}
	return -1
}

func TestOSFS_BasicOps(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		arrange func(t *testing.T) (*impl.OSFS, string)
		act    func(t *testing.T, fsys *impl.OSFS, path string) error
		assert assertPathFn
	}{
		{
			name: "mkdirall creates directory",
			arrange: func(t *testing.T) (*impl.OSFS, string) {
				dir := t.TempDir()
				return impl.New(), filepath.Join(dir, "a", "b")
			},
			act: func(t *testing.T, fsys *impl.OSFS, path string) error { return fsys.MkdirAll(path, fs.FileMode(0o755)) },
			assert: assertDirExists(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys, path := tc.arrange(t)
			err := tc.act(t, fsys, path)
			tc.assert(t, path, err)
		})
	}
}

func TestOSFS_ErrorPropagation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		arrange func(t *testing.T) (*impl.OSFS, string)
		act    func(t *testing.T, fsys *impl.OSFS, path string) error
		assert assertFn
	}{
		{
			name: "mkdirall error when path is existing file",
			arrange: func(t *testing.T) (*impl.OSFS, string) {
				dir := t.TempDir()
				file := filepath.Join(dir, "file.txt")
				if writeErr := os.WriteFile(file, []byte("x"), 0o644); writeErr != nil {
					t.Fatalf("prep file: %v", writeErr)
				}
				return impl.New(), file
			},
			act: func(t *testing.T, fsys *impl.OSFS, path string) error { return fsys.MkdirAll(path, 0o755) },
			assert: assertErrorContains("not a directory"),
		},
		{
			name: "write and read file round-trip",
			arrange: func(t *testing.T) (*impl.OSFS, string) {
				dir := t.TempDir()
				return impl.New(), filepath.Join(dir, "hello.txt")
			},
			act: func(t *testing.T, fsys *impl.OSFS, path string) error {
				ctx := context.Background()
				if err := fsys.WriteFile(ctx, path, []byte("hi"), 0o644); err != nil { return err }
				b, err := fsys.ReadFile(ctx, path)
				if err != nil { return err }
				if string(b) != "hi" { return &rtErr{got: string(b)} }
				return nil
			},
			assert: assertNoError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys, path := tc.arrange(t)
			err := tc.act(t, fsys, path)
			tc.assert(t, err)
		})
	}
}

type rtErr struct{ got string }

func (e *rtErr) Error() string { return "round-trip mismatch: " + e.got }
