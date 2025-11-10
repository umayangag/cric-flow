package exportdataset_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/exportdataset"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/exportdataset"
)

type mockFS struct {
	mkdirPath string
	mkdirPerm fs.FileMode
	mkdirErr  error
}

func (m *mockFS) ReadFile(_ context.Context, _ string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (m *mockFS) WriteFile(_ context.Context, _ string, _ []byte, _ fs.FileMode) error {
	return errors.New("not implemented")
}

func (m *mockFS) MkdirAll(path string, perm fs.FileMode) error {
	m.mkdirPath, m.mkdirPerm = path, perm
	return m.mkdirErr
}
func (m *mockFS) Glob(_ string) ([]string, error) { return nil, errors.New("not implemented") }

type assertFn func(t *testing.T, err error, m *mockFS)

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, err error, _ *mockFS) {
		if err == nil || !contains(err.Error(), sub) {
			t.Fatalf("want error containing %q, got %v", sub, err)
		}
	}
}

func assertNoErrorMkdirPath(want string) assertFn {
	return func(t *testing.T, err error, m *mockFS) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.mkdirPath != want {
			t.Fatalf("want mkdir path %q, got %q", want, m.mkdirPath)
		}
		if m.mkdirPerm != fs.FileMode(0o755) {
			t.Fatalf("want perm 0755, got %v", m.mkdirPerm)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	// simple substring search to avoid importing strings for the tiny helper
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func TestRunner_Run_MkdirAndValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		arrange func() (*cmd.Runner, cli.Options, *mockFS)
		assert  assertFn
	}{
		{
			name: "errors on empty outdir",
			arrange: func() (*cmd.Runner, cli.Options, *mockFS) {
				m := &mockFS{}
				r := cmd.NewRunner(m)
				return r, cli.Options{OutDir: ""}, m
			},
			assert: assertErrContains("output directory"),
		},
		{
			name: "creates outdir",
			arrange: func() (*cmd.Runner, cli.Options, *mockFS) {
				m := &mockFS{}
				r := cmd.NewRunner(m)
				return r, cli.Options{OutDir: "out"}, m
			},
			assert: assertNoErrorMkdirPath("out"),
		},
		{
			name: "propagates mkdir error",
			arrange: func() (*cmd.Runner, cli.Options, *mockFS) {
				m := &mockFS{mkdirErr: errors.New("boom")}
				r := cmd.NewRunner(m)
				return r, cli.Options{OutDir: "out"}, m
			},
			assert: assertErrContains("boom"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, opts, m := tc.arrange()
			err := r.Run(context.Background(), opts)
			tc.assert(t, err, m)
		})
	}
}
