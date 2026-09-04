package biography_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
)

// TestOpenCache_MissingFileIsAnEmptyCache: the first run has no cache by definition, and
// failing on that would make a clean-state run impossible.
func TestOpenCache_MissingFileIsAnEmptyCache(t *testing.T) {
	t.Parallel()

	cache, err := biography.OpenCache(filepath.Join(t.TempDir(), "nested", "lookups.jsonl"))

	require.NoError(t, err)
	assert.Zero(t, cache.Len())
}

// TestCachePutAndReopen_RemembersHitsAndMisses is what resumability is made of: a miss is
// an answer, and a cache that forgot misses would re-ask most of the ids every run.
func TestCachePutAndReopen_RemembersHitsAndMisses(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lookups.jsonl")
	cache, err := biography.OpenCache(path)
	require.NoError(t, err)

	require.NoError(t, cache.Put([]biography.Lookup{
		{CricinfoID: "35320", QID: "Q9200"},
		{CricinfoID: "999999"},
	}))
	reopened, err := biography.OpenCache(path)

	require.NoError(t, err)
	assert.Equal(t, 2, reopened.Len())
	hit, found := reopened.Get("35320")
	assert.True(t, found)
	assert.True(t, hit.Found())
	miss, asked := reopened.Get("999999")
	assert.True(t, asked, "the miss was asked, and asking it again would waste a request")
	assert.False(t, miss.Found())
	_, neverAsked := reopened.Get("1")
	assert.False(t, neverAsked)
}

// TestCachePut_SkipsAnEntryWithNoID keeps a keyless line out of the file, where it would
// be read back as an answer about nothing.
func TestCachePut_SkipsAnEntryWithNoID(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lookups.jsonl")
	cache, err := biography.OpenCache(path)
	require.NoError(t, err)

	require.NoError(t, cache.Put([]biography.Lookup{{QID: "Q1"}}))

	assert.Zero(t, cache.Len())
}

// TestCachePut_EmptyBatchWritesNothing avoids creating a file for a run that asked
// nothing.
func TestCachePut_EmptyBatchWritesNothing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lookups.jsonl")
	cache, err := biography.OpenCache(path)
	require.NoError(t, err)

	require.NoError(t, cache.Put(nil))

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

// TestOpenCache_DropsATornFinalLine is the interrupted-append case: losing one batch is
// the point of appending, where refusing to open the file would lose all of them.
func TestOpenCache_DropsATornFinalLine(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "lookups.jsonl")
	require.NoError(t, os.WriteFile(path,
		[]byte("{\"cricinfo_id\":\"35320\",\"qid\":\"Q9200\"}\n{\"cricinfo_id\":\"81"), 0o600))

	cache, err := biography.OpenCache(path)

	require.NoError(t, err)
	assert.Equal(t, 1, cache.Len())
}
