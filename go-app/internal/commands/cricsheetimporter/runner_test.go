package cricsheetimporter_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	cli "github.com/umayangag/cric-info-scrapers/go-app/internal/cli/cricsheetimporter"
	cmd "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/cricsheetimporter"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/models"
)

type memFS struct{}

func (memFS) ReadFile(context.Context, string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (memFS) WriteFile(context.Context, string, []byte, fs.FileMode) error {
	return errors.New("not implemented")
}
func (memFS) MkdirAll(string, fs.FileMode) error { return nil }
func (memFS) Glob(string) ([]string, error)      { return nil, errors.New("not implemented") }

type fakeLoader struct {
	ids  []string
	data map[string][]byte
	err  error
}

func (f *fakeLoader) List(_ context.Context, _ string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ids, nil
}

func (f *fakeLoader) Load(_ context.Context, _ string, id string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.data[id], nil
}

type fakeParser struct {
	matches map[string][]models.Match
	err     error
}

func (p *fakeParser) Parse(_ context.Context, raw []byte) ([]models.Match, error) {
	if p.err != nil {
		return nil, p.err
	}
	// In tests, raw contains key as string
	return p.matches[string(raw)], nil
}

type fakeRepo struct {
	got []models.Match
	err error
}

func (r *fakeRepo) UpsertMatches(_ context.Context, ms []models.Match) error {
	if r.err != nil {
		return r.err
	}
	r.got = append([]models.Match(nil), ms...)
	return nil
}

type assertFn func(t *testing.T, repo *fakeRepo, err error)

func assertNoErrorUpsertCount(n int) assertFn {
	return func(t *testing.T, repo *fakeRepo, err error) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(repo.got) != n {
			t.Fatalf("want upsert %d matches, got %d", n, len(repo.got))
		}
	}
}

func assertErrContains(sub string) assertFn {
	return func(t *testing.T, _ *fakeRepo, err error) {
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

func TestRunner_Run_BasicFlows(t *testing.T) {
	t.Parallel()
	ids := []string{"a.json", "b.json"}
	loader := &fakeLoader{ids: ids, data: map[string][]byte{"a.json": []byte("a"), "b.json": []byte("b")}}
	parser := &fakeParser{
		matches: map[string][]models.Match{"a": {{ID: 1, Format: "T20"}}, "b": {{ID: 2, Format: "ODI"}}},
	}
	repo := &fakeRepo{}
	r := cmd.NewRunner(memFS{}, loader, parser, repo, nil)
	opts := cli.Options{InDir: "/tmp", Apply: true, Concurrency: 1}
	err := r.Run(context.Background(), opts)
	assertNoErrorUpsertCount(2)(t, repo, err)
}

func TestRunner_Run_DryRun(t *testing.T) {
	to := t
	to.Parallel()
	loader := &fakeLoader{ids: []string{"x"}, data: map[string][]byte{"x": []byte("x")}}
	parser := &fakeParser{matches: map[string][]models.Match{"x": {{ID: 9, Format: "TEST"}}}}
	repo := &fakeRepo{}
	r := cmd.NewRunner(memFS{}, loader, parser, repo, nil)
	opts := cli.Options{InDir: "/tmp", Apply: false, Concurrency: 1}
	err := r.Run(context.Background(), opts)
	// dry-run should not call repo
	assertNoErrorUpsertCount(0)(t, repo, err)
}

func TestRunner_Run_Errors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		arrange func() (*cmd.Runner, cli.Options, *fakeRepo)
		assert  assertFn
	}{
		{
			name: "missing input dir",
			arrange: func() (*cmd.Runner, cli.Options, *fakeRepo) {
				return cmd.NewRunner(
					memFS{},
					&fakeLoader{},
					&fakeParser{},
					&fakeRepo{},
					nil,
				), cli.Options{}, &fakeRepo{}
			},
			assert: assertErrContains("input directory"),
		},
		{
			name: "list error",
			arrange: func() (*cmd.Runner, cli.Options, *fakeRepo) {
				fl := &fakeLoader{err: errors.New("boom")}
				return cmd.NewRunner(
						memFS{},
						fl,
						&fakeParser{},
						&fakeRepo{},
						nil,
					), cli.Options{
						InDir:       "/tmp",
						Apply:       true,
						Concurrency: 1,
					}, &fakeRepo{}
			},
			assert: assertErrContains("list inputs"),
		},
		{
			name: "parse error",
			arrange: func() (*cmd.Runner, cli.Options, *fakeRepo) {
				fl := &fakeLoader{ids: []string{"a"}, data: map[string][]byte{"a": []byte("a")}}
				fp := &fakeParser{err: errors.New("bad parse")}
				fr := &fakeRepo{}
				return cmd.NewRunner(
						memFS{},
						fl,
						fp,
						fr,
						nil,
					), cli.Options{
						InDir:       "/tmp",
						Apply:       true,
						Concurrency: 1,
					}, fr
			},
			assert: assertErrContains("parse"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, opts, repo := tc.arrange()
			err := r.Run(context.Background(), opts)
			tc.assert(t, repo, err)
		})
	}
}

func TestRunner_Run_MissingDepsAndNil(t *testing.T) {
	to := t
	to.Parallel()
	cases := []struct {
		name   string
		r      *cmd.Runner
		opts   cli.Options
		assert assertFn
	}{
		{"nil runner", nil, cli.Options{InDir: "/tmp"}, assertErrContains("nil runner")},
		{"missing deps", &cmd.Runner{}, cli.Options{InDir: "/tmp"}, assertErrContains("missing dependency")},
	}
	for _, tc := range cases {
		to.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.r == nil {
				err = (*cmd.Runner)(nil).Run(context.Background(), tc.opts)
			} else {
				err = tc.r.Run(context.Background(), tc.opts)
			}
			tc.assert(t, nil, err)
		})
	}
}

// loader that lists ok but fails on Load
type loadErrLoader struct{ ids []string }

func (l *loadErrLoader) List(_ context.Context, _ string) ([]string, error) { return l.ids, nil }
func (l *loadErrLoader) Load(_ context.Context, _ string, _ string) ([]byte, error) {
	return nil, errors.New("load boom")
}

func TestRunner_Run_LoadErrorAndEmptyList(t *testing.T) {
	t.Parallel()
	// load error case
	{
		loader := &loadErrLoader{ids: []string{"a"}}
		repo := &fakeRepo{}
		r := cmd.NewRunner(memFS{}, loader, &fakeParser{}, repo, nil)
		opts := cli.Options{InDir: "/tmp", Apply: true, Concurrency: 1}
		err := r.Run(context.Background(), opts)
		assertErrContains("load")(t, repo, err)
	}
	// empty list case
	{
		loader := &fakeLoader{ids: []string{}, data: map[string][]byte{}}
		repo := &fakeRepo{}
		r := cmd.NewRunner(memFS{}, loader, &fakeParser{}, repo, nil)
		opts := cli.Options{InDir: "/tmp", Apply: true, Concurrency: 1}
		err := r.Run(context.Background(), opts)
		// should succeed and upsert 0 matches
		assertNoErrorUpsertCount(0)(t, repo, err)
	}
}

func TestRunner_Run_UpsertError(t *testing.T) {
	t.Parallel()
	loader := &fakeLoader{ids: []string{"a"}, data: map[string][]byte{"a": []byte("a")}}
	parser := &fakeParser{matches: map[string][]models.Match{"a": {models.Match{ID: 1}}}}
	repo := &fakeRepo{err: errors.New("db fail")}
	r := cmd.NewRunner(memFS{}, loader, parser, repo, nil)
	opts := cli.Options{InDir: "/tmp", Apply: true, Concurrency: 1}
	err := r.Run(context.Background(), opts)
	assertErrContains("upsert matches")(t, repo, err)
}
