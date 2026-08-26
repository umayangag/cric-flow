package dataacquire

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stageArchive(t *testing.T, dir, name string, modified time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("PK\x03\x04"), 0o600))
	if !modified.IsZero() {
		require.NoError(t, os.Chtimes(path, modified, modified))
	}
	return path
}

func TestListStaged_NewestFirstAndArchivesOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := time.Now()
	stageArchive(t, dir, "older.zip", now.Add(-2*time.Hour))
	stageArchive(t, dir, "newer.zip", now)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "extract-123"), 0o755))

	staged := ListStaged(dir)
	require.Len(t, staged, 2, "only .zip files are archives")
	assert.Equal(t, "newer.zip", staged[0].Filename, "newest first")
	assert.Equal(t, "older.zip", staged[1].Filename)
}

func TestListStaged_MissingDirectoryIsEmptyNotAnError(t *testing.T) {
	t.Parallel()
	assert.Empty(t, ListStaged(filepath.Join(t.TempDir(), "never-fetched")))
}

// TestListStaged_CarriesProvenanceFromTheSidecar: the fetch recorded where the
// archive came from, and that is exactly what the operator needs to see before
// replacing the live dataset with it.
func TestListStaged_CarriesProvenanceFromTheSidecar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	archive := stageArchive(t, dir, "all_json.zip", time.Now())
	writeSidecar(archive, Result{
		SHA256:    "deadbeef",
		SourceURL: "https://cricsheet.org/downloads/all_json.zip",
		FeedID:    "all",
		FetchedAt: "2026-08-26T12:00:00Z",
	})

	staged := ListStaged(dir)
	require.Len(t, staged, 1)
	assert.Equal(t, "deadbeef", staged[0].SHA256)
	assert.Equal(t, "all", staged[0].FeedID)
	assert.Contains(t, staged[0].SourceURL, "cricsheet.org")
}

// TestResolveStagedArchive_RefusesAnythingButAPlainArchiveName: the name arrives in a
// request body, so it is checked here rather than trusted by whatever opens it.
func TestResolveStagedArchive_RefusesAnythingButAPlainArchiveName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stageArchive(t, dir, "all_json.zip", time.Now())

	bad := []string{
		"../../etc/shadow",
		"../all_json.zip",
		"sub/all_json.zip",
		`..\all_json.zip`,
		"/etc/passwd",
		"matches.json",
		"all_json.zip.txt",
		"missing.zip",
	}
	for _, name := range bad {
		_, err := ResolveStagedArchive(dir, name)
		assert.Error(t, err, "%q must be refused", name)
	}
}

func TestResolveStagedArchive_DefaultsToTheNewest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := time.Now()
	stageArchive(t, dir, "older.zip", now.Add(-time.Hour))
	newest := stageArchive(t, dir, "newer.zip", now)

	path, err := ResolveStagedArchive(dir, "")
	require.NoError(t, err)
	assert.Equal(t, newest, path)

	named, err := ResolveStagedArchive(dir, "older.zip")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "older.zip"), named)
}

func TestResolveStagedArchive_SaysSoWhenNothingIsStaged(t *testing.T) {
	t.Parallel()
	_, err := ResolveStagedArchive(t.TempDir(), "")
	require.ErrorIs(t, err, ErrNoStagedArchive)
}

// TestResolveStagedArchive_RefusesADirectoryNamedLikeAnArchive: the extraction
// workspace is created inside staging, so a directory there is not far-fetched.
func TestResolveStagedArchive_RefusesADirectoryNamedLikeAnArchive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "all_json.zip"), 0o755))

	_, err := ResolveStagedArchive(dir, "all_json.zip")
	require.Error(t, err)
}
