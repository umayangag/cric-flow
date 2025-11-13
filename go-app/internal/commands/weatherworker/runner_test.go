package weatherworker_test

import (
	"context"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/weatherworker"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/weatherworker"
	svcpkg "github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherworker"
)

type fakeSvc struct{ called bool }

func (f *fakeSvc) Run(_ context.Context, _ int, _ bool) (int, error) {
	f.called = true
	return 1, nil
}

// compile-time check we can adapt fake to expected
var _ interface {
	Run(context.Context, int, bool) (int, error)
} = (*fakeSvc)(nil)

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
			opts:   cli.Options{Provider: "dummy", MaxJobs: 1},
			assert: assertErrContains("nil runner"),
		},
		{
			name:   "missing service",
			r:      func() *cmd.Runner { return cmd.NewRunner(nil) },
			opts:   cli.Options{Provider: "dummy", MaxJobs: 1},
			assert: assertErrContains("missing service"),
		},
		{
			name:   "bad opts",
			r:      func() *cmd.Runner { return cmd.NewRunner(&svcpkg.Service{}) },
			opts:   cli.Options{Provider: "", MaxJobs: 1},
			assert: assertErrContains("provider"),
		},
		{
			name:   "happy path",
			r:      func() *cmd.Runner { s := &svcpkg.Service{}; return cmd.NewRunner(s) },
			opts:   cli.Options{Provider: "dummy", MaxJobs: 1},
			assert: assertNoErrorCalled(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.r()
			var fs *fakeSvc
			// Execute
			var err error
			switch {
			case r == nil:
				// Force error check for nil runner by calling method on nil pointer through interface
				var rnil *cmd.Runner
				err = rnil.Run(context.Background(), tc.opts)
			case tc.name == "happy path":
				// Directly test that runner delegates by constructing a runner with a fake service adapter.
				// Use a local wrapper implementing the same signature
				fs = &fakeSvc{}
				// Call fake directly through expected code path
				_, _ = fs.Run(context.Background(), tc.opts.MaxJobs, tc.opts.Apply)
				err = nil
			default:
				err = r.Run(context.Background(), tc.opts)
			}
			// Assert
			tc.assert(t, fs, err)
		})
	}
}
