package fielding_test

import (
	"context"
	"errors"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	fsvc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/fielding"
)

type fakeRepo struct {
	events    []db.BackfillEvent
	listErr   error
	upsertErr error
	upserts   [][]db.FieldingAggregateRow
}

func (r *fakeRepo) ListFieldingEvents(_ context.Context, matchID *int64) ([]db.BackfillEvent, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	if matchID == nil {
		return append([]db.BackfillEvent(nil), r.events...), nil
	}
	out := make([]db.BackfillEvent, 0, len(r.events))
	for _, e := range r.events {
		if e.MatchID == *matchID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r *fakeRepo) UpsertFieldingAggregates(_ context.Context, rows []db.FieldingAggregateRow) error {
	if r.upsertErr != nil {
		return r.upsertErr
	}
	cp := append([]db.FieldingAggregateRow(nil), rows...)
	r.upserts = append(r.upserts, cp)
	return nil
}

type assertSvcFn func(t *testing.T, n int, repo *fakeRepo, err error)

func assertNoErrorCount(want int, wantBatches int) assertSvcFn {
	return func(t *testing.T, n int, repo *fakeRepo, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if n != want {
			t.Fatalf("want count=%d got %d", want, n)
		}
		if len(repo.upserts) != wantBatches {
			t.Fatalf("want upsert batches=%d got %d", wantBatches, len(repo.upserts))
		}
	}
}

func assertErrorContains(sub string) assertSvcFn {
	return func(t *testing.T, _ int, _ *fakeRepo, err error) {
		s := ""
		if err != nil {
			s = err.Error()
		}
		if err == nil || indexOf(s, sub) < 0 {
			t.Fatalf("want err containing %q got %v", sub, err)
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

func TestService_BackfillMatch(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{events: []db.BackfillEvent{
		{MatchID: 1, PlayerID: 10, Catches: 1},
		{MatchID: 1, PlayerID: 10, RunOuts: 2},
		{MatchID: 1, PlayerID: 11, Stumpings: 1},
		{MatchID: 2, PlayerID: 10, Catches: 1},
	}}
	s := fsvc.NewService(repo)
	cases := []struct {
		name      string
		apply     bool
		match     int64
		listErr   error
		upsertErr error
		assert    assertSvcFn
	}{
		{"dry-run aggregates without upsert", false, 1, nil, nil, assertNoErrorCount(2, 0)},
		{"apply aggregates and upserts", true, 1, nil, nil, assertNoErrorCount(2, 1)},
		{"invalid match id", true, 0, nil, nil, assertErrorContains("invalid match id")},
		{"list error", true, 1, errors.New("boom"), nil, assertErrorContains("boom")},
		{"upsert error", true, 1, nil, errors.New("disk"), assertErrorContains("disk")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo.listErr = tc.listErr
			repo.upsertErr = tc.upsertErr
			n, err := s.BackfillMatch(context.Background(), int64(tc.match), tc.apply)
			tc.assert(t, n, repo, err)
		})
	}
}

func TestService_BackfillAll_ConcurrencyAndErrors(t *testing.T) {
	t.Parallel()
	events := []db.BackfillEvent{
		{MatchID: 1, PlayerID: 10, Catches: 1},
		{MatchID: 1, PlayerID: 11, RunOuts: 1},
		{MatchID: 2, PlayerID: 10, Stumpings: 1},
		{MatchID: 2, PlayerID: 12, RunoutsDirectHits: 1},
	}
	repo := &fakeRepo{events: events}
	s := fsvc.NewService(repo)
	cases := []struct {
		name      string
		apply     bool
		conc      int
		listErr   error
		upsertErr error
		assert    assertSvcFn
	}{
		{"dry-run counts total without upsert", false, 2, nil, nil, assertNoErrorCount(4, 0)},
		{"apply upserts in batches per match", true, 2, nil, nil, assertNoErrorCount(4, 2)},
		{"bad concurrency", true, 0, nil, nil, assertErrorContains("concurrency")},
		{"list error", true, 1, errors.New("boom"), nil, assertErrorContains("boom")},
		{"upsert error", true, 1, nil, errors.New("fail"), assertErrorContains("fail")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo.listErr = tc.listErr
			repo.upsertErr = tc.upsertErr
			repo.upserts = nil
			n, err := s.BackfillAll(context.Background(), tc.apply, tc.conc)
			tc.assert(t, n, repo, err)
		})
	}
}
