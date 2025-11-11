package teampredictor_test

import (
	"context"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/teampredictor"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/teampredictor"
	svcpkg "github.com/umayangag/cric-info-scrapers/go-app/internal/services/teampredictor"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/mlclient"
)

type fakeSvc struct { out mlclient.PredictResponse }

func (f *fakeSvc) Predict(_ context.Context, _ cli.Options) (mlclient.PredictResponse, error) { return f.out, nil }

// Compile-time assurance this shape exists on real service, here we just need the method
var _ interface{ Predict(context.Context, cli.Options) (mlclient.PredictResponse, error) } = (*fakeSvc)(nil)

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

func TestRunner_Run(t *testing.T) {
	t.Parallel()
	cases := []struct{
		name string
		r func() *cmd.Runner
		opts cli.Options
		assert assertFn
	}{
		{
			name: "nil runner",
			r: func() *cmd.Runner { return nil },
			opts: cli.Options{MatchID:1,Format:"T20",Season:"2019"},
			assert: assertErrContains("nil runner"),
		},
		{
			name: "missing service",
			r: func() *cmd.Runner { return cmd.NewRunner(nil) },
			opts: cli.Options{MatchID:1,Format:"T20",Season:"2019"},
			assert: assertErrContains("missing service"),
		},
		{
			name: "invalid opts",
			r: func() *cmd.Runner { return cmd.NewRunner(&svcpkg.Service{}) },
			opts: cli.Options{MatchID:0,Format:"T20",Season:"2019"},
			assert: assertErrContains("invalid options"),
		},
		{
			name: "happy path",
			r: func() *cmd.Runner { return cmd.NewRunner(&svcpkg.Service{}) },
			opts: cli.Options{MatchID:1,Format:"T20",Season:"2019"},
			assert: assertNoError(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T){
			r := tc.r()
			var err error
			if r == nil {
				// Force error check for nil runner by calling method on nil pointer through interface
				var rnil *cmd.Runner
				_, err = rnil.Run(context.Background(), tc.opts)
			} else {
				_, err = r.Run(context.Background(), tc.opts)
			}
			tc.assert(t, err)
		})
	}
}
