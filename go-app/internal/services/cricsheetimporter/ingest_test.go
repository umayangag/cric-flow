package cricsheetimporter_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/domain"
	svc "github.com/umayangag/cric-info-scrapers/go-app/internal/services/cricsheetimporter"
)

type fakeLoader struct {
	list []string
	load map[string][]byte
	err  error
	mu   sync.Mutex
	seen []string
}

func (f *fakeLoader) List(_ context.Context, _ string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]string, len(f.list))
	copy(out, f.list)
	return out, nil
}

func (f *fakeLoader) Load(_ context.Context, _ string, id string) ([]byte, error) {
	f.mu.Lock()
	f.seen = append(f.seen, id)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	b, ok := f.load[id]
	if !ok {
		return nil, fmt.Errorf("missing: %s", id)
	}
	return b, nil
}

type fakeParser struct {
	out map[string][]domain.Match
	err error
}

func (p *fakeParser) Parse(_ context.Context, raw []byte) ([]domain.Match, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.out[string(raw)], nil
}

type fakeRepo struct {
	mu      sync.Mutex
	upserts [][]domain.Match
	err     error
}

func (r *fakeRepo) UpsertMatches(_ context.Context, m []domain.Match) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.upserts = append(r.upserts, append([]domain.Match(nil), m...))
	return nil
}

// assert helpers (no ifs in test bodies)

type assertSvcFn func(t *testing.T, processed int, err error, fl *fakeLoader, fr *fakeRepo)

func assertNoErrorProcessed(want int) assertSvcFn {
	return func(t *testing.T, processed int, err error, _ *fakeLoader, _ *fakeRepo) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if processed != want {
			t.Fatalf("want processed=%d got=%d", want, processed)
		}
	}
}

func assertErrContains(sub string) assertSvcFn {
	return func(t *testing.T, _ int, err error, _ *fakeLoader, _ *fakeRepo) {
		s := ""
		if err != nil {
			s = err.Error()
		}
		if err == nil || indexOf(s, sub) < 0 {
			t.Fatalf("want err containing %q, got %v", sub, err)
		}
	}
}

func assertRepoBatches(want int) assertSvcFn {
	return func(t *testing.T, _ int, err error, _ *fakeLoader, r *fakeRepo) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(r.upserts) != want {
			t.Fatalf("want %d upsert batches, got %d", want, len(r.upserts))
		}
	}
}

func assertLoaderSaw(ids ...string) assertSvcFn {
	return func(t *testing.T, _ int, err error, fl *fakeLoader, _ *fakeRepo) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		got := append([]string(nil), fl.seen...)
		sort.Strings(got)
		sort.Strings(ids)
		if len(got) != len(ids) {
			t.Fatalf("loader saw %v, want %v", got, ids)
		}
		for i := range ids {
			if got[i] != ids[i] {
				t.Fatalf("loader saw %v, want %v", got, ids)
			}
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

func TestIngestService_BasicFlows(t *testing.T) {
	t.Parallel()
	// common fakes
	fl := &fakeLoader{
		list: []string{"a.json", "b.json"},
		load: map[string][]byte{"a.json": []byte("A"), "b.json": []byte("B")},
	}
	fp := &fakeParser{out: map[string][]domain.Match{
		"A": {{ID: 1}},
		"B": {{ID: 2}, {ID: 3}},
	}}
	fr := &fakeRepo{}
	s := &svc.IngestService{Loader: fl, Parser: fp, Repository: fr}

	cases := []struct {
		name   string
		apply  bool
		conc   int
		assert assertSvcFn
	}{
		{"dry-run single worker", false, 1, func(t *testing.T, p int, e error, fl *fakeLoader, fr *fakeRepo) {
			assertNoErrorProcessed(2)(t, p, e, fl, fr)
			assertRepoBatches(0)(t, p, e, fl, fr)
			assertLoaderSaw("a.json", "b.json")(t, p, e, fl, fr)
		}},
		{"apply with 2 workers", true, 2, func(t *testing.T, p int, e error, fl *fakeLoader, fr *fakeRepo) {
			assertNoErrorProcessed(2)(t, p, e, fl, fr)
			assertRepoBatches(2)(t, p, e, fl, fr)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			processed, err := s.IngestDir(context.Background(), ".", tc.apply, tc.conc)
			tc.assert(t, processed, err, fl, fr)
		})
	}
}

func TestIngestService_Errors(t *testing.T) {
	t.Parallel()
	mk := func() (*svc.IngestService, *fakeLoader, *fakeRepo) {
		fl := &fakeLoader{list: []string{"x.json"}, load: map[string][]byte{"x.json": []byte("X")}}
		fp := &fakeParser{out: map[string][]domain.Match{"X": {{ID: 9}}}}
		fr := &fakeRepo{}
		return &svc.IngestService{Loader: fl, Parser: fp, Repository: fr}, fl, fr
	}
	cases := []struct {
		name   string
		arr    func() (*svc.IngestService, *fakeLoader, *fakeRepo)
		dir    string
		assert assertSvcFn
	}{
		{
			"nil deps",
			func() (*svc.IngestService, *fakeLoader, *fakeRepo) { return &svc.IngestService{}, nil, nil },
			".",
			assertErrContains("nil service"),
		},
		{
			"empty dir",
			func() (*svc.IngestService, *fakeLoader, *fakeRepo) { s, fl, fr := mk(); return s, fl, fr },
			"",
			assertErrContains("input directory"),
		},
		{"list error", func() (*svc.IngestService, *fakeLoader, *fakeRepo) {
			s, fl, fr := mk()
			fl.err = errors.New("boom")
			return s, fl, fr
		}, ".", assertErrContains("boom")},
		{"load error", func() (*svc.IngestService, *fakeLoader, *fakeRepo) {
			s, fl, fr := mk()
			fl.err = nil
			s.Loader = &fakeLoader{list: []string{"y.json"}, load: map[string][]byte{}, err: nil}
			return s, s.Loader.(*fakeLoader), fr
		}, ".", assertErrContains("missing")},
		{"parse error", func() (*svc.IngestService, *fakeLoader, *fakeRepo) {
			s, fl, fr := mk()
			s.Parser = &fakeParser{err: errors.New("parse")}
			return s, fl, fr
		}, ".", assertErrContains("parse")},
		{"repo error", func() (*svc.IngestService, *fakeLoader, *fakeRepo) {
			s, fl, fr := mk()
			fr.err = errors.New("db")
			return s, fl, fr
		}, ".", assertErrContains("db")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, fl, fr := tc.arr()
			_, err := s.IngestDir(context.Background(), tc.dir, true, 1)
			tc.assert(t, 0, err, fl, fr)
		})
	}
}
