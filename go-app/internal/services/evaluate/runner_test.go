package evaluate_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	svc "github.com/umayangag/cric-flow/go-app/internal/services/evaluate"

	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	in  svc.Inputs
	err error
}

func (f fakeRepo) LoadInputs(_ context.Context, _, _ string) (svc.Inputs, error) {
	if f.err != nil {
		return svc.Inputs{}, f.err
	}
	return f.in, nil
}

func TestRunner_Run(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		repo    svc.EvaluationRepo
		opts    svc.Options
		wantSub string
		wantErr bool
	}{
		{
			name: "happy path formats metrics",
			repo: fakeRepo{in: svc.Inputs{
				YTrue: []float64{1, 2},
				YPred: []float64{1.5, 2.5},
				YWin:  []float64{1, 0},
				YProb: []float64{0.9, 0.1},
			}},
			opts:    svc.Options{Season: "2019", Format: "T20"},
			wantSub: "MAE=",
		},
		{
			name:    "repo error propagates",
			repo:    fakeRepo{err: errors.New("boom")},
			opts:    svc.Options{Season: "2019", Format: "T20"},
			wantErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			r := svc.Runner{Repo: tc.repo, Out: buf}
			err := r.Run(context.Background(), tc.opts)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Contains(t, buf.String(), tc.wantSub)
		})
	}
}
