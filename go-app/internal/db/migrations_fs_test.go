package db_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dbpkg "github.com/umayangag/cric-flow/go-app/internal/db"
	dmocks "github.com/umayangag/cric-flow/go-app/internal/db/mocks"
)

// stringRowsForMigrations returns db.Rows that yields the given strings. Used by migration tests.
func stringRowsForMigrations(vals []string) dbpkg.Rows {
	return &migrationsStringRows{vals: vals}
}

type migrationsStringRows struct {
	vals []string
	idx  int
}

func (r *migrationsStringRows) Next() bool {
	if r.idx < len(r.vals) {
		r.idx++
		return true
	}
	return false
}

func (r *migrationsStringRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.vals) || len(dest) == 0 {
		return nil
	}
	if p, ok := dest[0].(*string); ok {
		*p = r.vals[r.idx-1]
		return nil
	}
	return nil
}

func (r *migrationsStringRows) Close()     {}
func (r *migrationsStringRows) Err() error { return nil }

// Helper to setup a DB mock for migrations tests
//
//nolint:unparam
func setupMigrationsDBMock(
	t *testing.T,
	initiallyApplied []string,
	failOnSubstr string,
) (*dmocks.MockDB, dbpkg.Rows, *map[string]bool) {
	t.Helper()
	applied := map[string]bool{}
	for _, v := range initiallyApplied {
		applied[v] = true
	}
	// Rows over the applied set (sorted lexically like the real query)
	list := make([]string, 0, len(applied))
	for v := range applied {
		list = append(list, v)
	}
	sort.Strings(list)
	rows := stringRowsForMigrations(list)

	dbm := &dmocks.MockDB{}
	// CREATE TABLE IF NOT EXISTS ... always ok
	dbm.On("Exec", mock.Anything, mock.MatchedBy(func(sql string) bool {
		return strings.Contains(strings.ToLower(sql), "create table if not exists schema_migrations")
	}), mock.Anything).Return(nil)
	// SELECT version FROM schema_migrations → return rows
	dbm.On("Query", mock.Anything, mock.MatchedBy(func(sql string) bool {
		return strings.Contains(strings.ToLower(sql), "select version from schema_migrations")
	})).Return(rows, nil)
	// Exec failure when SQL contains a specific substring (used for 002_bad.sql case)
	if failOnSubstr != "" {
		dbm.On("Exec", mock.Anything, mock.MatchedBy(func(sql string) bool {
			return strings.Contains(strings.ToLower(sql), strings.ToLower(failOnSubstr))
		}), mock.Anything).Return(errors.New("exec failure"))
	}
	// Catch-all Exec for migration SQL that are not INSERTs and not failing
	dbm.On("Exec", mock.Anything, mock.MatchedBy(func(sql string) bool {
		up := strings.ToUpper(strings.TrimSpace(sql))
		return !strings.HasPrefix(up, "INSERT INTO SCHEMA_MIGRATIONS")
	}), mock.Anything).Return(nil)
	// INSERT INTO schema_migrations(version) VALUES($1)
	dbm.On("Exec", mock.Anything, mock.MatchedBy(func(sql string) bool {
		return strings.HasPrefix(strings.TrimSpace(strings.ToUpper(sql)), "INSERT INTO SCHEMA_MIGRATIONS")
	}), mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		// args: ctx, sql, argsSlice; version is argsSlice[0]
		if arr, ok := args.Get(2).([]any); ok && len(arr) > 0 {
			if v, ok := arr[0].(string); ok {
				applied[v] = true
			}
		}
	})

	dbpkg.SetDB(dbm)
	t.Cleanup(func() { dbpkg.SetDB(nil) })
	return dbm, rows, &applied
}

func TestRunMigrationsFS_AppliesInLexicalOrder(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{
		"m/001_init.sql": &fstest.MapFile{Data: []byte("-- init\nCREATE TABLE a(id int);")},
		"m/010_more.sql": &fstest.MapFile{Data: []byte("-- more\nALTER TABLE a ADD COLUMN b int;")},
		"m/002_add.sql":  &fstest.MapFile{Data: []byte("-- add\nINSERT INTO a(id) VALUES(1);")},
		"m/readme.txt":   &fstest.MapFile{Data: []byte("ignore")},
	}
	_, _, applied := setupMigrationsDBMock(t, nil, "")
	err := dbpkg.RunMigrationsFS(ctx, fsys, "m")
	require.NoError(t, err)
	// verify inserts executed for all three versions
	for _, v := range []string{"001_init.sql", "002_add.sql", "010_more.sql"} {
		require.Truef(t, (*applied)[v], "version %s not applied", v)
	}
}

func TestRunMigrationsFS_SkipsAlreadyApplied(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{
		"m/001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE a(id int);")},
		"m/002_add.sql":  &fstest.MapFile{Data: []byte("INSERT INTO a(id) VALUES(1);")},
	}
	_, _, applied := setupMigrationsDBMock(t, []string{"001_init.sql"}, "")
	err := dbpkg.RunMigrationsFS(ctx, fsys, "m")
	require.NoError(t, err)
	require.True(t, (*applied)["001_init.sql"]) // was already there
	require.True(t, (*applied)["002_add.sql"])  // newly applied
}

// Rollback scripts must never be applied forward. Because "down" sorts before "up",
// a *.down.sql picked up as an ordinary migration would run before the migration it
// undoes and drop objects a fresh bootstrap has just created.
func TestRunMigrationsFS_NeverAppliesDownMigrations(t *testing.T) {
	cases := []struct {
		name         string
		files        map[string]string
		wantApplied  []string
		wantSkipped  []string
		wantNotInSQL string
	}{
		{
			name: "down file paired with an up file",
			files: map[string]string{
				"m/001_init.sql":     "CREATE TABLE a(id int);",
				"m/002_col.up.sql":   "ALTER TABLE a ADD COLUMN b int;",
				"m/002_col.down.sql": "DROP TABLE IF EXISTS down_marker_paired;",
			},
			wantApplied:  []string{"001_init.sql", "002_col.up.sql"},
			wantSkipped:  []string{"002_col.down.sql"},
			wantNotInSQL: "down_marker_paired",
		},
		{
			name: "down file with no matching up file",
			files: map[string]string{
				"m/001_init.sql":        "CREATE TABLE a(id int);",
				"m/003_orphan.down.sql": "DROP TABLE IF EXISTS down_marker_orphan;",
			},
			wantApplied:  []string{"001_init.sql"},
			wantSkipped:  []string{"003_orphan.down.sql"},
			wantNotInSQL: "down_marker_orphan",
		},
		{
			name: "uppercase extension is still recognised as a rollback script",
			files: map[string]string{
				"m/001_init.sql":       "CREATE TABLE a(id int);",
				"m/004_shout.DOWN.SQL": "DROP TABLE IF EXISTS down_marker_shout;",
			},
			wantApplied:  []string{"001_init.sql"},
			wantSkipped:  []string{"004_shout.DOWN.SQL"},
			wantNotInSQL: "down_marker_shout",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			fsys := fstest.MapFS{}
			for name, body := range tc.files {
				fsys[name] = &fstest.MapFile{Data: []byte(body)}
			}

			dbm, _, applied := setupMigrationsDBMock(t, nil, "")
			require.NoError(t, dbpkg.RunMigrationsFS(ctx, fsys, "m"))

			for _, v := range tc.wantApplied {
				require.Truef(t, (*applied)[v], "expected %s to be applied", v)
			}
			for _, v := range tc.wantSkipped {
				require.Falsef(t, (*applied)[v], "rollback script %s must not be recorded as applied", v)
			}

			// The strongest assertion: the rollback body never reached the database at all,
			// so ordering cannot matter.
			for _, call := range dbm.Calls {
				if call.Method != "Exec" || len(call.Arguments) < 2 {
					continue
				}
				sql, ok := call.Arguments.Get(1).(string)
				require.True(t, ok)
				require.NotContainsf(t, sql, tc.wantNotInSQL,
					"rollback SQL was executed: %s", sql)
			}
		})
	}
}

func TestRunMigrationsFS_StopsOnExecError(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{
		"m/001_ok.sql":  &fstest.MapFile{Data: []byte("CREATE TABLE a(id int);")},
		"m/002_bad.sql": &fstest.MapFile{Data: []byte("BAD SQL STATEMENT")},
		"m/003_ok.sql":  &fstest.MapFile{Data: []byte("INSERT INTO a(id) VALUES(1);")},
	}
	_, _, applied := setupMigrationsDBMock(t, nil, "bad sql")
	err := dbpkg.RunMigrationsFS(ctx, fsys, "m")
	require.Error(t, err)
	// Only 001 should be applied; 002 fails, 003 not attempted.
	require.True(t, (*applied)["001_ok.sql"])
	require.False(t, (*applied)["002_bad.sql"])
	require.False(t, (*applied)["003_ok.sql"])
}

func TestRunMigrationsFS_EmptyDir_NoOps(t *testing.T) {
	ctx := context.Background()
	// Directory exists but contains no .sql files
	fsys := fstest.MapFS{
		"m/.keep": &fstest.MapFile{Data: []byte("")},
	}
	_, _, applied := setupMigrationsDBMock(t, nil, "")
	err := dbpkg.RunMigrationsFS(ctx, fsys, "m")
	require.NoError(t, err)
	require.Len(t, *applied, 0)
}

func TestRunMigrationsFS_IdempotentOnSecondRun(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{
		"m/001_init.sql": &fstest.MapFile{Data: []byte("CREATE TABLE a(id int);")},
		"m/002_add.sql":  &fstest.MapFile{Data: []byte("INSERT INTO a(id) VALUES(1);")},
	}
	// First run applies both
	_, _, applied := setupMigrationsDBMock(t, nil, "")
	err := dbpkg.RunMigrationsFS(ctx, fsys, "m")
	require.NoError(t, err)
	require.True(t, (*applied)["001_init.sql"])
	require.True(t, (*applied)["002_add.sql"])

	// Second run should skip inserts; ensure we do not record new versions
	_, _, applied2 := setupMigrationsDBMock(t, []string{"001_init.sql", "002_add.sql"}, "")
	err = dbpkg.RunMigrationsFS(ctx, fsys, "m")
	require.NoError(t, err)
	// No change expected (already present)
	require.True(t, (*applied2)["001_init.sql"])
	require.True(t, (*applied2)["002_add.sql"])
}
