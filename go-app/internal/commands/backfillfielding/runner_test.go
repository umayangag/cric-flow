package backfillfielding_test

import (
	"context"
	"errors"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/backfillfielding"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/backfillfielding"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	fsvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/fielding"
)

type fakeRepo struct {
	lastMatchPtr *int64
	listErr      error
	upsertErr    error
}

func (r *fakeRepo) ListFieldingEvents(_ context.Context, matchID *int64) ([]db.BackfillEvent, error) {
	r.lastMatchPtr = matchID
	if r.listErr != nil {
		return nil, r.listErr
	}
	// return minimal single player single match events to keep service logic simple
	mid := int64(1)
	pid := int64(7)
	if matchID != nil {
		mid = *matchID
	}
	return []db.BackfillEvent{{MatchID: mid, PlayerID: pid, Catches: 1}}, nil
}

func (r *fakeRepo) UpsertFieldingAggregates(_ context.Context, _ []db.FieldingAggregateRow) error {
	return r.upsertErr
}

type assertFn func(t *testing.T, err error, fr *fakeRepo)

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, err error, _ *fakeRepo) {
		s := ""
		if err != nil { s = err.Error() }
		if err == nil || indexOf(s, sub) < 0 {
			t.Fatalf("want err containing %q, got %v", sub, err)
		}
	}
}

func assertNoErrorAndAllUsed() assertFn {
	return func(t *testing.T, err error, fr *fakeRepo) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if fr.lastMatchPtr != nil {
			t.Fatalf("expected nil match ptr for --all path, got non-nil")
		}
	}
}

func assertNoErrorAndMatchUsed(want int64) assertFn {
	return func(t *testing.T, err error, fr *fakeRepo) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if fr.lastMatchPtr == nil || *fr.lastMatchPtr != want {
			t.Fatalf("expected match ptr %d, got %v", want, fr.lastMatchPtr)
		}
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] { ok = false; break }
		}
		if ok { return i }
	}
	return -1
}

func TestRunner_Run_Table(t *testing.T) {
	t.Parallel()

	cases := []struct{
		name   string
		arrange func() (*cmd.Runner, cli.Options, *fakeRepo)
		assert assertFn
	}{
		{
			name: "nil service errors",
			arrange: func() (*cmd.Runner, cli.Options, *fakeRepo) {
				return &cmd.Runner{Svc: nil}, cli.Options{All: true, Apply: false, Concurrency: 1}, nil
			},
			assert: assertErrContains("missing service"),
		},
		{
			name: "all path calls BackfillAll (nil match ptr)",
			arrange: func() (*cmd.Runner, cli.Options, *fakeRepo) {
				fr := &fakeRepo{}
				s := fsvc.NewService(fr)
				r := cmd.NewRunner(s)
				return r, cli.Options{All: true, Apply: false, Concurrency: 1}, fr
			},
			assert: assertNoErrorAndAllUsed(),
		},
		{
			name: "match path calls BackfillMatch (non-nil match ptr)",
			arrange: func() (*cmd.Runner, cli.Options, *fakeRepo) {
				fr := &fakeRepo{}
				s := fsvc.NewService(fr)
				r := cmd.NewRunner(s)
				return r, cli.Options{MatchID: 42, Apply: false}, fr
			},
			assert: assertNoErrorAndMatchUsed(42),
		},
		{
			name: "invalid match id",
			arrange: func() (*cmd.Runner, cli.Options, *fakeRepo) {
				fr := &fakeRepo{}
				s := fsvc.NewService(fr)
				r := cmd.NewRunner(s)
				return r, cli.Options{MatchID: 0}, fr
			},
			assert: assertErrContains("invalid match id"),
		},
		{
			name: "service error propagates",
			arrange: func() (*cmd.Runner, cli.Options, *fakeRepo) {
				fr := &fakeRepo{listErr: errors.New("boom")}
				s := fsvc.NewService(fr)
				r := cmd.NewRunner(s)
				return r, cli.Options{All: true, Apply: false, Concurrency: 1}, fr
			},
			assert: assertErrContains("boom"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, opts, fr := tc.arrange()
			err := r.Run(context.Background(), opts)
			tc.assert(t, err, fr)
		})
	}
}
