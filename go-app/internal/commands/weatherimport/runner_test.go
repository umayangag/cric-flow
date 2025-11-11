package weatherimport_test

import (
	"context"
	"errors"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/weatherimport"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/weatherimport"
)

type fakeSvc struct{ n int; err error }

func (s *fakeSvc) Import(_ context.Context, _ int64, _ bool) (int, error) {
	return s.n, s.err
}

type assertFn func(t *testing.T, err error)

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, err error) {
		s := ""; if err != nil { s = err.Error() }
		if err == nil || indexOf(s, sub) < 0 { t.Fatalf("want err containing %q got %v", sub, err) }
	}
}

func assertNoError() assertFn {
	return func(t *testing.T, err error) {
		if err != nil { t.Fatalf("unexpected err: %v", err) }
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

func TestRunner_Run_Table(t *testing.T) {
	t.Parallel()

	cases := []struct{
		name string
		arrange func() (*cmd.Runner, cli.Options)
		assert assertFn
	}{
		{
			name: "nil service errors",
			arrange: func() (*cmd.Runner, cli.Options) { return &cmd.Runner{Svc: nil}, cli.Options{MatchID: 1, Apply: false} },
			assert: assertErrContains("missing service"),
		},
		{
			name: "invalid match id",
			arrange: func() (*cmd.Runner, cli.Options) {
				r := cmd.NewRunner(&fakeSvc{})
				return r, cli.Options{MatchID: 0, Apply: false}
			},
			assert: assertErrContains("invalid match id"),
		},
		{
			name: "happy path delegates to service",
			arrange: func() (*cmd.Runner, cli.Options) {
				r := cmd.NewRunner(&fakeSvc{n: 2})
				return r, cli.Options{MatchID: 42, Apply: true}
			},
			assert: assertNoError(),
		},
		{
			name: "service error propagates",
			arrange: func() (*cmd.Runner, cli.Options) {
				r := cmd.NewRunner(&fakeSvc{err: errors.New("boom")})
				return r, cli.Options{MatchID: 7, Apply: true}
			},
			assert: assertErrContains("boom"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, opts := tc.arrange()
			err := r.Run(context.Background(), opts)
			tc.assert(t, err)
		})
	}
}
