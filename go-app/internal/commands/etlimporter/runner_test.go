package etlimporter_test

import (
	"context"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/etlimporter"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/etlimporter"
	svcpkg "github.com/umayangag/cric-info-scrapers/go-app/internal/services/etlimporter"
)

type fakeSvc struct{ called bool }

func (f *fakeSvc) IngestDir(_ context.Context, _ string, _ string, _ bool, _ int) (svcpkg.Stats, error) {
	f.called = true
	return svcpkg.Stats{Files: 1}, nil
}

type assertFn func(t *testing.T, svc *fakeSvc, err error)

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, _ *fakeSvc, err error) {
		s := ""
		if err != nil {
			s = err.Error()
		}
		if err == nil || indexOf(s, sub) < 0 {
			t.Fatalf("want err containing %q, got %v", sub, err)
		}
	}
}

func assertNoErrorCalled() assertFn {
	return func(t *testing.T, svc *fakeSvc, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !svc.called {
			t.Fatalf("expected service to be called")
		}
	}
}

func indexOf(s, sub string) int {
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

func TestRunner_Run(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		r      func() *cmd.Runner
		opts   cli.Options
		assert assertFn
	}{
		{
			name:   "nil runner",
			r:      func() *cmd.Runner { return nil },
			opts:   cli.Options{InDir: "/x", Concurrency: 1, Pattern: "*.csv"},
			assert: assertErrContains("nil runner"),
		},
		{
			name: "missing service",
			r: func() *cmd.Runner { return cmd.NewRunner(nil) },
			opts:   cli.Options{InDir: "/x", Concurrency: 1, Pattern: "*.csv"},
			assert: assertErrContains("missing service"),
		},
		{
			name:   "bad opts",
			r:      func() *cmd.Runner { return cmd.NewRunner(&fakeSvc{}) },
			opts:   cli.Options{InDir: "", Concurrency: 0, Pattern: ""},
			assert: assertErrContains("input directory"),
		},
		{
			name:   "happy path calls service",
			r:      func() *cmd.Runner { return cmd.NewRunner(&fakeSvc{}) },
			opts:   cli.Options{InDir: "/data", Concurrency: 1, Pattern: "*.csv", Apply: true},
			assert: assertNoErrorCalled(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.r()
			err := r.Run(context.Background(), tc.opts)
			var svc *fakeSvc
			if r != nil {
				// best-effort: type assert when our fake is used
				if fs, ok := r.Svc.(*fakeSvc); ok {
					svc = fs
				}
			}
			tc.assert(t, svc, err)
		})
	}
}
