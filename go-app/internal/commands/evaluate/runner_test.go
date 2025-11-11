package evaluate_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	clieval "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/evaluate"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/evaluate"
	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/evaluate"
)

type fakeRepo struct{ in svc.Inputs; err error }

func (f fakeRepo) LoadInputs(ctx context.Context, season, format string) (svc.Inputs, error) {
	if f.err != nil { return svc.Inputs{}, f.err }
	return f.in, nil
}

type assertRunFn func(t *testing.T, out string, err error)

func assertRunSuccessContains(sub string) assertRunFn {
	return func(t *testing.T, out string, err error) {
		if err != nil { t.Fatalf("unexpected err: %v", err) }
		if !contains(out, sub) { t.Fatalf("output %q does not contain %q", out, sub) }
	}
}

func assertRunError() assertRunFn {
	return func(t *testing.T, _ string, err error) {
		if err == nil { t.Fatalf("expected error, got nil") }
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ { if s[i+j] != sub[j] { ok = false; break } }
		if ok { return true }
	}
	return false
}

func TestRunner_Run(t *testing.T) {
	t.Parallel()
	cases := []struct{
		name string
		repo cmd.Runner
		opts clieval.Options
		assert assertRunFn
	}{
		{
			name: "happy path formats metrics",
			repo: cmd.Runner{Repo: fakeRepo{in: svc.Inputs{YTrue: []float64{1,2}, YPred: []float64{1.5, 2.5}, YWin: []float64{1,0}, YProb: []float64{0.9,0.1}}}},
			opts: clieval.Options{Season:"2019", Format:"T20"},
			assert: assertRunSuccessContains("MAE="),
		},
		{
			name: "repo error propagates",
			repo: cmd.Runner{Repo: fakeRepo{err: errors.New("boom")}},
			opts: clieval.Options{Season:"2019", Format:"T20"},
			assert: assertRunError(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			r := tc.repo
			r.Out = buf
			err := r.Run(context.Background(), tc.opts)
			tc.assert(t, buf.String(), err)
		})
	}
}
