package datasetregistry

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/mocks"
)

// conflictClause returns the DO UPDATE half of an upsert, so a test can assert on
// what a conflict changes without also matching the INSERT column list above it.
func conflictClause(t *testing.T, query string) string {
	t.Helper()
	_, after, found := strings.Cut(query, "DO UPDATE")
	require.True(t, found, "expected an upsert, got: %s", query)
	return after
}

func withMockDB(t *testing.T) *mocks.MockDB {
	t.Helper()
	mockDB := mocks.NewMockDB(t)
	db.SetDB(mockDB)
	t.Cleanup(func() { db.SetDB(nil) })
	return mockDB
}

// TestRecordFetch_UpsertsOnTheDigest states the identity rule. Cricsheet reuses
// filenames across releases, so the same name is not the same dataset — and the same
// bytes arriving twice are not two datasets.
func TestRecordFetch_UpsertsOnTheDigest(t *testing.T) {
	mockDB := withMockDB(t)

	var gotSQL string
	var gotArgs []any
	mockDB.EXPECT().Exec(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, query string, args ...any) error {
			gotSQL, gotArgs = query, args
			return nil
		}).Once()

	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, RecordFetch(context.Background(), FetchRecord{
		SHA256:    "abc123",
		Feed:      "t20s",
		SourceURL: "https://cricsheet.org/downloads/t20s_json.zip",
		Filename:  "t20s_json.zip",
		Bytes:     4096,
		ETag:      `"v1"`,
		FetchedAt: now,
	}))

	assert.Contains(t, gotSQL, "ON CONFLICT (sha256)", "identical bytes are one dataset, however often they arrive")
	assert.Equal(t, "abc123", gotArgs[0])
	assert.Equal(t, "t20s", gotArgs[1])
}

// TestRecordFetch_DoesNotClearTheExtractColumns: re-downloading an archive that is
// already extracted must not erase the record that it was.
func TestRecordFetch_DoesNotClearTheExtractColumns(t *testing.T) {
	mockDB := withMockDB(t)

	var gotSQL string
	mockDB.EXPECT().Exec(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, query string, _ ...any) error {
			gotSQL = query
			return nil
		}).Once()

	require.NoError(t, RecordFetch(context.Background(), FetchRecord{SHA256: "abc", Filename: "a.zip"}))

	update := conflictClause(t, gotSQL)
	for _, column := range []string{"entry_count", "match_files", "extracted_at", "extracted_bytes", "dest_dir"} {
		assert.NotContains(t, update, column,
			"a re-fetch must not touch %s: the extraction still happened", column)
	}
}

// TestRecordExtract_InsertsForAHandPlacedArchive: extract must be able to create the
// row, not only update one, because an archive can reach staging without a fetch.
func TestRecordExtract_InsertsForAHandPlacedArchive(t *testing.T) {
	mockDB := withMockDB(t)

	var gotSQL string
	var gotArgs []any
	mockDB.EXPECT().Exec(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, query string, args ...any) error {
			gotSQL, gotArgs = query, args
			return nil
		}).Once()

	require.NoError(t, RecordExtract(context.Background(), ExtractRecord{
		SHA256:     "deadbeef",
		Filename:   "all_json.zip",
		EntryCount: 20000,
		MatchFiles: 19998,
		DestDir:    "/data/cricsheet",
	}))

	assert.Contains(t, gotSQL, "INSERT INTO datasets")
	assert.Contains(t, gotSQL, "ON CONFLICT (sha256)")
	assert.Equal(t, "deadbeef", gotArgs[0])
}

// TestRecordExtract_KeepsExistingProvenance: an extract knows less about origin than
// the fetch did, so it must not overwrite feed and URL with its own blanks.
func TestRecordExtract_KeepsExistingProvenance(t *testing.T) {
	mockDB := withMockDB(t)

	var gotSQL string
	mockDB.EXPECT().Exec(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, query string, _ ...any) error {
			gotSQL = query
			return nil
		}).Once()

	require.NoError(t, RecordExtract(context.Background(), ExtractRecord{SHA256: "abc", Filename: "a.zip"}))

	update := conflictClause(t, gotSQL)
	assert.Contains(t, update, "COALESCE(datasets.feed, EXCLUDED.feed)")
	assert.Contains(t, update, "COALESCE(datasets.source_url, EXCLUDED.source_url)")
}

// TestRecord_SkipsWithoutADigest: the digest is the primary key, so a record without
// one has nothing to be. Writing it would violate the NOT NULL and fail the job for
// something the job did correctly.
func TestRecord_SkipsWithoutADigest(t *testing.T) {
	withMockDB(t) // no Exec expectation: nothing may be written

	require.NoError(t, RecordFetch(context.Background(), FetchRecord{Filename: "a.zip"}))
	require.NoError(t, RecordExtract(context.Background(), ExtractRecord{Filename: "a.zip"}))
}

// TestRecord_IsInertWithoutADatabase: the ops console runs against a box whose
// Postgres may be down, and a registry write must not be what turns a working
// download into a failed one.
func TestRecord_IsInertWithoutADatabase(t *testing.T) {
	db.SetDB(nil)
	require.NoError(t, RecordFetch(context.Background(), FetchRecord{SHA256: "abc", Filename: "a.zip"}))
	require.NoError(t, RecordExtract(context.Background(), ExtractRecord{SHA256: "abc", Filename: "a.zip"}))

	got, err := List(context.Background(), 10, "")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestRecordFetch_ReturnsTheDatabaseError(t *testing.T) {
	mockDB := withMockDB(t)
	mockDB.EXPECT().Exec(mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("connection refused")).Once()

	err := RecordFetch(context.Background(), FetchRecord{SHA256: "abc", Filename: "a.zip"})
	require.Error(t, err, "the caller decides whether to log or fail; this layer reports")
}

// datasetRow feeds scanDataset one row of values.
type datasetRow struct {
	values []any
}

func (r *datasetRow) Scan(dest ...any) error {
	if len(dest) != len(r.values) {
		return errors.New("column count mismatch")
	}
	for i, d := range dest {
		switch p := d.(type) {
		case *int64:
			*p = r.values[i].(int64)
		case *string:
			*p = r.values[i].(string)
		case *sql.NullString:
			*p = r.values[i].(sql.NullString)
		case *sql.NullInt64:
			*p = r.values[i].(sql.NullInt64)
		case *sql.NullTime:
			*p = r.values[i].(sql.NullTime)
		case *time.Time:
			*p = r.values[i].(time.Time)
		default:
			return errors.New("unsupported destination")
		}
	}
	return nil
}

// fullRow builds a row with every optional column present.
func fullRow(sha string) []any {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	return []any{
		int64(1), sha,
		sql.NullString{String: "all", Valid: true},
		sql.NullString{String: "https://cricsheet.org/downloads/all_json.zip", Valid: true},
		"all_json.zip", int64(4096),
		sql.NullString{String: `"v1"`, Valid: true},
		sql.NullString{String: "Wed, 26 Aug 2026 07:28:00 GMT", Valid: true},
		sql.NullTime{Time: now, Valid: true},
		sql.NullTime{Time: now, Valid: true},
		sql.NullInt64{Int64: 20000, Valid: true},
		sql.NullInt64{Int64: 19998, Valid: true},
		sql.NullInt64{Int64: 1 << 30, Valid: true},
		sql.NullString{String: "/data/cricsheet", Valid: true},
		now, now,
	}
}

// bareRow builds a row for a hand-placed, never-extracted archive: every optional
// column null.
func bareRow(sha string) []any {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	return []any{
		int64(2), sha,
		sql.NullString{},
		sql.NullString{},
		"mystery.zip", int64(10),
		sql.NullString{},
		sql.NullString{},
		sql.NullTime{},
		sql.NullTime{},
		sql.NullInt64{},
		sql.NullInt64{},
		sql.NullInt64{},
		sql.NullString{},
		now, now,
	}
}

// mockRowsFor returns a MockRows that yields the given rows in order.
func mockRowsFor(t *testing.T, rows [][]any) *mocks.MockRows {
	t.Helper()
	mockRows := mocks.NewMockRows(t)
	i := 0
	mockRows.EXPECT().Next().RunAndReturn(func() bool { return i < len(rows) }).Maybe()
	mockRows.EXPECT().Scan(mock.Anything).RunAndReturn(func(dest ...any) error {
		row := &datasetRow{values: rows[i]}
		i++
		return row.Scan(dest...)
	}).Maybe()
	mockRows.EXPECT().Close().Return().Maybe()
	mockRows.EXPECT().Err().Return(nil).Maybe()
	return mockRows
}

// TestList_MarksTheLiveDataset is the plan's third bullet: the registry has to say
// which of these is the one in the data directory.
func TestList_MarksTheLiveDataset(t *testing.T) {
	mockDB := withMockDB(t)
	mockDB.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).
		Return(mockRowsFor(t, [][]any{fullRow("live-sha"), bareRow("other-sha")}), nil).Once()

	got, err := List(context.Background(), 10, "live-sha")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.True(t, got[0].Live)
	assert.False(t, got[1].Live)
}

// TestList_MarksNothingLiveWhenTheDirectoryHasNoManifest: a hand-populated data
// directory has genuinely unknown provenance, and no row may claim to be it.
func TestList_MarksNothingLiveWhenTheDirectoryHasNoManifest(t *testing.T) {
	mockDB := withMockDB(t)
	mockDB.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).
		Return(mockRowsFor(t, [][]any{fullRow("some-sha")}), nil).Once()

	got, err := List(context.Background(), 10, "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.False(t, got[0].Live, "an empty live digest must not match a row")
}

// TestList_RendersAbsentColumnsAsAbsent: "we do not know where this came from" and
// "it came from nowhere" are different answers, and the second is never true.
func TestList_RendersAbsentColumnsAsAbsent(t *testing.T) {
	mockDB := withMockDB(t)
	mockDB.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).
		Return(mockRowsFor(t, [][]any{bareRow("mystery-sha")}), nil).Once()

	got, err := List(context.Background(), 10, "")
	require.NoError(t, err)
	require.Len(t, got, 1)

	d := got[0]
	assert.Empty(t, d.Feed)
	assert.Empty(t, d.SourceURL)
	assert.Empty(t, d.FetchedAt, "a hand-placed archive was never fetched")
	assert.Empty(t, d.ExtractedAt)
	assert.Zero(t, d.EntryCount)
	assert.Equal(t, "mystery.zip", d.Filename)
}

func TestList_ReadsFullRows(t *testing.T) {
	mockDB := withMockDB(t)
	mockDB.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).
		Return(mockRowsFor(t, [][]any{fullRow("sha")}), nil).Once()

	got, err := List(context.Background(), 10, "")
	require.NoError(t, err)
	require.Len(t, got, 1)

	d := got[0]
	assert.Equal(t, "all", d.Feed)
	assert.Equal(t, 20000, d.EntryCount)
	assert.Equal(t, 19998, d.MatchFiles)
	assert.Equal(t, "/data/cricsheet", d.DestDir)
	assert.Equal(t, "2026-08-26T12:00:00Z", d.FetchedAt)
}

func TestList_DefaultsAndBoundsTheLimit(t *testing.T) {
	mockDB := withMockDB(t)
	var gotLimit any
	mockDB.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, args ...any) (db.Rows, error) {
			gotLimit = args[0]
			return mockRowsFor(t, nil), nil
		}).Once()

	_, err := List(context.Background(), 0, "")
	require.NoError(t, err)
	assert.Equal(t, 50, gotLimit, "a missing limit must not mean unbounded")
}

func TestList_ReturnsTheQueryError(t *testing.T) {
	mockDB := withMockDB(t)
	mockDB.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("connection refused")).Once()

	_, err := List(context.Background(), 10, "")
	require.Error(t, err)
}
