package datasetregistry

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// These exercise the SQL itself. The unit tests above use a mocked DB, which proves
// the queries are *sent* but not that Postgres accepts them — an ON CONFLICT clause
// naming a column that is not unique, or a COALESCE over the wrong table alias, would
// pass every one of them and fail on the box. Run with:
//
//	RUN_DB_TESTS=1 make -C go-app test-db
func guardIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_DB_TESTS") != "1" {
		t.Skip("integration test skipped; set RUN_DB_TESTS=1 to run")
	}
}

// migrationsDir resolves go-app/migrations from this file's location, so the test
// works whatever working directory go test picks.
func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../migrations"))
}

// setupRegistryDB connects, migrates and empties the table so each test starts clean.
func setupRegistryDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	_, err := db.Connect(ctx)
	require.NoError(t, err, "integration tests need POSTGRES_* pointing at a database")
	require.NoError(t, db.RunMigrations(ctx, migrationsDir()))
	require.NoError(t, db.Exec(ctx, `TRUNCATE datasets`))
}

func TestRecordAndList_Integration(t *testing.T) {
	guardIntegration(t)
	setupRegistryDB(t)
	ctx := context.Background()

	fetchedAt := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	require.NoError(t, RecordFetch(ctx, FetchRecord{
		SHA256:       "aaa111",
		Feed:         "t20s",
		SourceURL:    "https://cricsheet.org/downloads/t20s_json.zip",
		Filename:     "t20s_json.zip",
		Bytes:        4096,
		ETag:         `"v1"`,
		LastModified: "Wed, 26 Aug 2026 07:28:00 GMT",
		FetchedAt:    fetchedAt,
	}))

	got, err := List(ctx, 10, "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "t20s", got[0].Feed)
	assert.Equal(t, int64(4096), got[0].Bytes)
	assert.Equal(t, "2026-08-26T10:00:00Z", got[0].FetchedAt)
	assert.Empty(t, got[0].ExtractedAt, "a fetched-but-not-extracted archive has no extract time")
	assert.False(t, got[0].Live)
}

// TestRefetchDoesNotDuplicateOrEraseTheExtract_Integration is the behaviour the whole
// upsert design exists for, checked against real Postgres rather than a string match.
func TestRefetchDoesNotDuplicateOrEraseTheExtract_Integration(t *testing.T) {
	guardIntegration(t)
	setupRegistryDB(t)
	ctx := context.Background()

	require.NoError(t, RecordFetch(ctx, FetchRecord{
		SHA256: "bbb222", Feed: "all", SourceURL: "https://cricsheet.org/downloads/all_json.zip",
		Filename: "all_json.zip", Bytes: 100, FetchedAt: time.Now().UTC(),
	}))
	require.NoError(t, RecordExtract(ctx, ExtractRecord{
		SHA256: "bbb222", Filename: "all_json.zip",
		EntryCount: 20000, MatchFiles: 19998, ExtractedBytes: 1 << 20,
		DestDir: "/data/cricsheet", ExtractedAt: time.Now().UTC(),
	}))

	// The operator clicks Fetch again; the bytes are unchanged.
	require.NoError(t, RecordFetch(ctx, FetchRecord{
		SHA256: "bbb222", Feed: "all", SourceURL: "https://cricsheet.org/downloads/all_json.zip",
		Filename: "all_json.zip", Bytes: 100, FetchedAt: time.Now().UTC(),
	}))

	got, err := List(ctx, 10, "")
	require.NoError(t, err)
	require.Len(t, got, 1, "identical bytes are one dataset, however often they arrive")
	assert.Equal(t, 20000, got[0].EntryCount, "a re-fetch must not erase the extraction record")
	assert.Equal(t, 19998, got[0].MatchFiles)
	assert.Equal(t, "/data/cricsheet", got[0].DestDir)
	assert.NotEmpty(t, got[0].ExtractedAt)
}

// TestExtractKeepsFetchProvenance_Integration: an extract knows less about origin
// than the fetch did, so it must not overwrite feed and URL with its own blanks.
func TestExtractKeepsFetchProvenance_Integration(t *testing.T) {
	guardIntegration(t)
	setupRegistryDB(t)
	ctx := context.Background()

	require.NoError(t, RecordFetch(ctx, FetchRecord{
		SHA256: "ccc333", Feed: "ipl", SourceURL: "https://cricsheet.org/downloads/ipl_json.zip",
		Filename: "ipl_json.zip", Bytes: 50, FetchedAt: time.Now().UTC(),
	}))
	// Extract with no feed or URL, as happens when the sidecar is gone.
	require.NoError(t, RecordExtract(ctx, ExtractRecord{
		SHA256: "ccc333", Filename: "ipl_json.zip", EntryCount: 10, MatchFiles: 10,
		DestDir: "/data/cricsheet", ExtractedAt: time.Now().UTC(),
	}))

	got, err := List(ctx, 10, "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "ipl", got[0].Feed, "the extract must not blank out what the fetch knew")
	assert.Contains(t, got[0].SourceURL, "cricsheet.org")
}

// TestExtractOfAHandPlacedArchive_Integration: extract has to be able to create the
// row, since an archive can reach staging without ever passing through fetch.
func TestExtractOfAHandPlacedArchive_Integration(t *testing.T) {
	guardIntegration(t)
	setupRegistryDB(t)
	ctx := context.Background()

	require.NoError(t, RecordExtract(ctx, ExtractRecord{
		SHA256: "ddd444", Filename: "mystery.zip", EntryCount: 5, MatchFiles: 5,
		DestDir: "/data/cricsheet", ExtractedAt: time.Now().UTC(),
	}))

	got, err := List(ctx, 10, "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Empty(t, got[0].Feed, "an archive nobody fetched has no feed, and says so")
	assert.Empty(t, got[0].SourceURL)
	assert.Empty(t, got[0].FetchedAt)
	assert.Equal(t, 5, got[0].MatchFiles)
}

func TestListMarksTheLiveDataset_Integration(t *testing.T) {
	guardIntegration(t)
	setupRegistryDB(t)
	ctx := context.Background()

	for _, sha := range []string{"eee555", "fff666"} {
		require.NoError(t, RecordFetch(ctx, FetchRecord{
			SHA256: sha, Filename: sha + ".zip", Bytes: 1, FetchedAt: time.Now().UTC(),
		}))
	}

	got, err := List(ctx, 10, "fff666")
	require.NoError(t, err)
	require.Len(t, got, 2)

	live := 0
	for _, d := range got {
		if d.Live {
			live++
			assert.Equal(t, "fff666", d.SHA256)
		}
	}
	assert.Equal(t, 1, live, "exactly one dataset is in the data directory")
}

func TestListOrdersNewestFirst_Integration(t *testing.T) {
	guardIntegration(t)
	setupRegistryDB(t)
	ctx := context.Background()

	require.NoError(t, RecordFetch(ctx, FetchRecord{SHA256: "old", Filename: "old.zip", FetchedAt: time.Now().UTC()}))
	// created_at defaults to now(), so a distinguishable gap is needed.
	require.NoError(t, db.Exec(ctx, `UPDATE datasets SET created_at = now() - interval '1 day' WHERE sha256 = 'old'`))
	require.NoError(t, RecordFetch(ctx, FetchRecord{SHA256: "new", Filename: "new.zip", FetchedAt: time.Now().UTC()}))

	got, err := List(ctx, 10, "")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "new", got[0].SHA256, "newest first")
}

func TestListRespectsTheLimit_Integration(t *testing.T) {
	guardIntegration(t)
	setupRegistryDB(t)
	ctx := context.Background()

	for i := range 5 {
		sha := string(rune('a'+i)) + "-limit"
		require.NoError(
			t,
			RecordFetch(ctx, FetchRecord{SHA256: sha, Filename: sha + ".zip", FetchedAt: time.Now().UTC()}),
		)
	}

	got, err := List(ctx, 2, "")
	require.NoError(t, err)
	assert.Len(t, got, 2)
}
