package weatherworker_test

import (
	"context"
	"errors"
	"testing"

	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/weatherworker"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/wx"
)

type fakeJobs struct {
	batches [][]int64
	i       int
	err     error
}

func (f *fakeJobs) Next(_ context.Context, _ int) ([]int64, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	if f.i >= len(f.batches) {
		return nil, false, nil
	}
	ids := f.batches[f.i]
	f.i++
	return ids, true, nil
}

type fakeProv struct {
	recs map[int64][]wx.Record
	err  error
}

func (p *fakeProv) Fetch(_ context.Context, id int64) ([]wx.Record, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.recs[id], nil
}

type fakeRepo struct {
	upserts int
	err     error
}

func (r *fakeRepo) UpsertWeather(_ context.Context, _ wx.Record) error {
	if r.err != nil {
		return r.err
	}
	r.upserts++
	return nil
}

type assertFn func(t *testing.T, processed int, repo *fakeRepo, err error)

func assertNoErrorProcessed(want int, wantUpserts int) assertFn {
	return func(t *testing.T, processed int, repo *fakeRepo, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if processed != want {
			t.Fatalf("want processed=%d got %d", want, processed)
		}
		if repo.upserts != wantUpserts {
			t.Fatalf("want upserts=%d got %d", wantUpserts, repo.upserts)
		}
	}
}

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, _ int, _ *fakeRepo, err error) {
		s := ""
		if err != nil {
			s = err.Error()
		}
		if err == nil || indexOf(s, sub) < 0 {
			t.Fatalf("want err containing %q, got %v", sub, err)
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

func TestService_Run_Table(t *testing.T) {
	t.Parallel()

	bat := func(id int64) []wx.Record { return []wx.Record{{MatchID: id, Session: "batting", Temp: 25}} }
	bow := func(id int64) []wx.Record { return []wx.Record{{MatchID: id, Session: "bowling", Temp: 24}} }

	cases := []struct {
		name   string
		jobs   *fakeJobs
		prov   *fakeProv
		repo   *fakeRepo
		max    int
		apply  bool
		assert assertFn
	}{
		{
			name:   "dry-run processes all without upserts",
			jobs:   &fakeJobs{batches: [][]int64{{1, 2}, {3}}},
			prov:   &fakeProv{recs: map[int64][]wx.Record{1: bat(1), 2: bow(2), 3: bat(3)}},
			repo:   &fakeRepo{},
			max:    0,
			apply:  false,
			assert: assertNoErrorProcessed(3, 0),
		},
		{
			name:   "apply upserts all records",
			jobs:   &fakeJobs{batches: [][]int64{{10}, {11}}},
			prov:   &fakeProv{recs: map[int64][]wx.Record{10: {bat(10)[0], bow(10)[0]}, 11: bat(11)}},
			repo:   &fakeRepo{},
			max:    0,
			apply:  true,
			assert: assertNoErrorProcessed(2, 3),
		},
		{
			name:   "respect max jobs",
			jobs:   &fakeJobs{batches: [][]int64{{1, 2, 3}}},
			prov:   &fakeProv{recs: map[int64][]wx.Record{1: bat(1), 2: bat(2), 3: bat(3)}},
			repo:   &fakeRepo{},
			max:    2,
			apply:  true,
			assert: assertNoErrorProcessed(2, 2),
		},
		{
			name:   "jobs error surfaces",
			jobs:   &fakeJobs{err: errors.New("boom")},
			prov:   &fakeProv{},
			repo:   &fakeRepo{},
			max:    0,
			apply:  false,
			assert: assertErrContains("boom"),
		},
		{
			name:   "provider error surfaces",
			jobs:   &fakeJobs{batches: [][]int64{{5}}},
			prov:   &fakeProv{err: errors.New("p")},
			repo:   &fakeRepo{},
			max:    0,
			apply:  false,
			assert: assertErrContains("p"),
		},
		{
			name:   "repo error surfaces",
			jobs:   &fakeJobs{batches: [][]int64{{7}}},
			prov:   &fakeProv{recs: map[int64][]wx.Record{7: {bat(7)[0]}}},
			repo:   &fakeRepo{err: errors.New("db")},
			max:    0,
			apply:  true,
			assert: assertErrContains("db"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := svc.NewService(tc.jobs, tc.prov, tc.repo)
			processed, err := s.Run(context.Background(), tc.max, tc.apply)
			tc.assert(t, processed, tc.repo, err)
		})
	}
}
