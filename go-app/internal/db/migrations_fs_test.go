package db

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

type fakeRows struct {
	vals []string
	i    int
}

func (r *fakeRows) Next() bool {
	if r.i < len(r.vals) {
		r.i++
		return true
	}
	return false
}
func (r *fakeRows) Scan(dest ...any) error {
	if r.i == 0 || r.i > len(r.vals) {
		return errors.New("scan out of range")
	}
	p, ok := dest[0].(*string)
	if !ok {
		return errors.New("dest type")
	}
	*p = r.vals[r.i-1]
	return nil
}
func (r *fakeRows) Close() {}

type fakeDB struct {
	applied map[string]bool
	execs   []string
	failOn  string // substring that triggers failure on Exec
}

func (f *fakeDB) Exec(ctx context.Context, sql string, args ...any) error {
	// record sql
	f.execs = append(f.execs, sql)
	// simulate failure
	if f.failOn != "" && strings.Contains(strings.ToLower(sql), strings.ToLower(f.failOn)) {
		return errors.New("exec failure")
	}
	// capture inserts into schema_migrations
	if strings.HasPrefix(strings.TrimSpace(strings.ToUpper(sql)), "INSERT INTO SCHEMA_MIGRATIONS") {
		if len(args) != 1 {
			return errors.New("insert requires version arg")
		}
		v, _ := args[0].(string)
		if f.applied == nil {
			f.applied = map[string]bool{}
		}
		f.applied[v] = true
	}
	return nil
}

func (f *fakeDB) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	// only query we support in migrations
	if !strings.Contains(strings.ToLower(sql), "select version from schema_migrations") {
		return nil, errors.New("unexpected query")
	}
	// deterministically list applied versions
	var list []string
	for v := range f.applied {
		list = append(list, v)
	}
	sort.Strings(list)
	return &fakeRows{vals: list}, nil
}

func TestRunMigrationsFS_AppliesInLexicalOrder(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{
		"m/001_init.sql": &fstest.MapFile{Data: []byte("-- init\nCREATE TABLE a(id int);")},
		"m/010_more.sql": &fstest.MapFile{Data: []byte("-- more\nALTER TABLE a ADD COLUMN b int;")},
		"m/002_add.sql":  &fstest.MapFile{Data: []byte("-- add\nINSERT INTO a(id) VALUES(1);")},
		"m/readme.txt":   &fstest.MapFile{Data: []byte("ignore")},
	}
	fdb := &fakeDB{applied: map[string]bool{}}
	SetDB(fdb)
	if err := RunMigrationsFS(ctx, fsys, "m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// verify inserts executed in lexical order 001,002,010
	want := []string{"001_init.sql", "002_add.sql", "010_more.sql"}
	for _, v := range want {
		if !fdb.applied[v] {
			t.Fatalf("version %s not applied", v)
		}
	}
}

func TestRunMigrationsFS_SkipsAlreadyApplied(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{
		"m/001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE a(id int);")},
		"m/002_add.sql":  &fstest.MapFile{Data: []byte("INSERT INTO a(id) VALUES(1);")},
	}
	fdb := &fakeDB{applied: map[string]bool{"001_init.sql": true}}
	SetDB(fdb)
	if err := RunMigrationsFS(ctx, fsys, "m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fdb.applied["001_init.sql"] || !fdb.applied["002_add.sql"] {
		t.Fatalf("expected 002_add.sql applied; got %#v", fdb.applied)
	}
}

func TestRunMigrationsFS_StopsOnExecError(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{
		"m/001_ok.sql":  &fstest.MapFile{Data: []byte("CREATE TABLE a(id int);")},
		"m/002_bad.sql": &fstest.MapFile{Data: []byte("BAD SQL STATEMENT")},
		"m/003_ok.sql":  &fstest.MapFile{Data: []byte("INSERT INTO a(id) VALUES(1);")},
	}
	fdb := &fakeDB{applied: map[string]bool{}, failOn: "bad sql"}
	SetDB(fdb)
	err := RunMigrationsFS(ctx, fsys, "m")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	// Only 001 should be applied; 002 fails, 003 not attempted.
	if !fdb.applied["001_ok.sql"] || fdb.applied["002_bad.sql"] || fdb.applied["003_ok.sql"] {
		t.Fatalf("unexpected applied set after failure: %#v", fdb.applied)
	}
}

func TestRunMigrationsFS_EmptyDir_NoOps(t *testing.T) {
	ctx := context.Background()
	// Directory exists but contains no .sql files
	fsys := fstest.MapFS{
		"m/.keep": &fstest.MapFile{Data: []byte("")},
	}
	fdb := &fakeDB{applied: map[string]bool{}}
	SetDB(fdb)
	if err := RunMigrationsFS(ctx, fsys, "m"); err != nil {
		t.Fatalf("unexpected error on empty dir: %v", err)
	}
	// No versions should be applied
	if len(fdb.applied) != 0 {
		t.Fatalf("expected no applied versions, got %#v", fdb.applied)
	}
}

func TestRunMigrationsFS_IdempotentOnSecondRun(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{
		"m/001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE a(id int);")},
		"m/002_add.sql":  &fstest.MapFile{Data: []byte("INSERT INTO a(id) VALUES(1);")},
	}
	fdb := &fakeDB{applied: map[string]bool{}}
	SetDB(fdb)
	if err := RunMigrationsFS(ctx, fsys, "m"); err != nil {
		t.Fatalf("first run error: %v", err)
	}
	// On second run, fail if any INSERT into schema_migrations is attempted
	fdb.execs = nil
	fdb.failOn = "insert into schema_migrations"
	if err := RunMigrationsFS(ctx, fsys, "m"); err != nil {
		t.Fatalf("unexpected error on idempotent second run: %v", err)
	}
}
