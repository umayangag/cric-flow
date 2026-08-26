package dataset

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile creates a file with the given contents and modification time.
func writeFile(t *testing.T, dir, name, contents string, modified time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	if !modified.IsZero() {
		require.NoError(t, os.Chtimes(path, modified, modified))
	}
	return path
}

func TestDirPrefersTheEnvironmentOverride(t *testing.T) {
	// Not parallel: mutates the environment.
	t.Setenv(DirEnvVar, "/tmp/somewhere-else")
	assert.Equal(t, "/tmp/somewhere-else", Dir())

	t.Setenv(DirEnvVar, "   ")
	assert.NotEqual(t, "   ", Dir(), "a blank override must not win")
	assert.NotEmpty(t, Dir())
}

func TestInspectCountsOnlyImportableFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	writeFile(t, dir, "1000.json", "aaaa", older)
	writeFile(t, dir, "1001.JSON", "bb", newer)
	writeFile(t, dir, "README.md", "ignored", newer)
	// Import does not recurse, so a nested match file must not be counted: claiming it
	// would promise data the importer will never read.
	nested := filepath.Join(dir, "nested")
	require.NoError(t, os.Mkdir(nested, 0o755))
	writeFile(t, nested, "2000.json", "cccccc", newer)

	inv := Inspect(dir)
	assert.True(t, inv.Exists)
	assert.True(t, inv.Readable)
	assert.Equal(t, 2, inv.MatchFiles)
	assert.Equal(t, int64(6), inv.Bytes)
	assert.Equal(t, "1001.JSON", inv.NewestFile)
	assert.Equal(t, "2026-06-01T00:00:00Z", inv.NewestModified)
	assert.False(t, inv.IsEmpty())
	assert.Empty(t, inv.Error)
}

func TestInspectReportsAnEmptyDirectoryRatherThanFailing(t *testing.T) {
	t.Parallel()

	inv := Inspect(t.TempDir())
	assert.True(t, inv.Exists)
	assert.True(t, inv.Readable)
	assert.True(t, inv.IsEmpty())
	assert.Empty(t, inv.Error)
}

func TestInspectReportsAMissingDirectory(t *testing.T) {
	t.Parallel()

	inv := Inspect(filepath.Join(t.TempDir(), "not-there"))
	assert.False(t, inv.Exists)
	assert.True(t, inv.IsEmpty())
	assert.NotEmpty(t, inv.Error, "the ops console needs to be told why")
}

func TestInspectReportsAPathThatIsNotADirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := writeFile(t, dir, "data", "not a directory", time.Time{})

	inv := Inspect(file)
	assert.False(t, inv.Exists)
	assert.Equal(t, "not a directory", inv.Error)
}

// TestMatchFilesAgreesWithInspect is the point of MatchFiles existing: the count the
// ops console shows and the list the importer reads come from one rule.
func TestMatchFilesAgreesWithInspect(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "b.json", "x", time.Time{})
	writeFile(t, dir, "a.json", "x", time.Time{})
	writeFile(t, dir, "notes.txt", "x", time.Time{})

	files, err := MatchFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json")}, files,
		"files must be sorted so imports are reproducible")
	assert.Len(t, files, Inspect(dir).MatchFiles)
}

func TestMatchFilesOnAMissingDirectory(t *testing.T) {
	t.Parallel()

	_, err := MatchFiles(filepath.Join(t.TempDir(), "not-there"))
	assert.Error(t, err)
}

// TestStagingDirIsInvisibleToImport is the property the staging location depends on:
// a downloaded archive sitting under the dataset directory must never be counted as
// match data, or the console would report files the importer will not read.
func TestStagingDirIsInvisibleToImport(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(DirEnvVar, dir)

	staging := StagingDir()
	require.Equal(t, filepath.Join(dir, StagingDirName), staging)
	require.NoError(t, os.MkdirAll(staging, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staging, "all_json.zip"), []byte("PK"), 0o600))
	// A stray .json inside staging: a partially-extracted archive, or an operator's
	// scratch file. Import does not recurse, so neither may the inventory.
	require.NoError(t, os.WriteFile(filepath.Join(staging, "match.json"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.json"), []byte("{}"), 0o600))

	inv := Inspect(dir)
	assert.Equal(t, 1, inv.MatchFiles, "only the file directly in the dataset directory counts")

	files, err := MatchFiles(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "real.json")}, files)
}
