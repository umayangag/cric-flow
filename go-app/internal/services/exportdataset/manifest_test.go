package exportdataset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteManifest_RecordsTheDatasetBehindTheExport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "batting_encoded_T20I.csv"), []byte("a,b\n1,2\n"), 0o600))

	writeManifest(dir, []string{"T20I", "ODI"}, true, Provenance{
		DatasetSHA256:    "deadbeef",
		DatasetSourceURL: "https://cricsheet.org/downloads/all_json.zip",
		DatasetFeed:      "all",
		DatasetExtracted: "2026-08-26T10:00:00Z",
		DatasetMatchFile: 19998,
	})

	got, ok := ReadManifest(dir)
	require.True(t, ok)
	assert.Equal(t, ManifestVersion, got.Version)
	assert.Equal(t, "deadbeef", got.Provenance.DatasetSHA256)
	assert.Equal(t, 19998, got.Provenance.DatasetMatchFile)
	assert.Equal(t, []string{"T20I", "ODI"}, got.Formats)
	assert.True(t, got.Unified)
	assert.NotEmpty(t, got.ExportedAt)
}

// TestWriteManifest_RecordsSizes: an empty CSV is a failed export that reported
// success — the shape of failure this repo keeps meeting — and a manifest that listed
// a file without its size would hide exactly that.
func TestWriteManifest_RecordsSizes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "full.csv"), []byte("a,b\n1,2\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "empty.csv"), nil, 0o600))

	writeManifest(dir, nil, false, Provenance{})

	got, ok := ReadManifest(dir)
	require.True(t, ok)
	byName := map[string]int64{}
	for _, f := range got.Files {
		byName[f.Name] = f.Bytes
	}
	assert.Positive(t, byName["full.csv"])
	assert.Zero(t, byName["empty.csv"], "an empty export is visible, not hidden")
}

func TestWriteManifest_ListsOnlyCSVs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "batting.csv"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o755))

	writeManifest(dir, nil, false, Provenance{})

	got, ok := ReadManifest(dir)
	require.True(t, ok)
	require.Len(t, got.Files, 1)
	assert.Equal(t, "batting.csv", got.Files[0].Name)
}

// TestWriteManifest_UnknownProvenanceIsRecordedAsUnknown: a data directory populated
// by hand genuinely has no answer, and inventing one would defeat P-2.
func TestWriteManifest_UnknownProvenanceIsRecordedAsUnknown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeManifest(dir, nil, false, Provenance{})

	got, ok := ReadManifest(dir)
	require.True(t, ok)
	assert.False(t, got.Provenance.Known())

	raw, err := os.ReadFile(ManifestPath(dir))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "dataset_sha256", "absent fields are omitted, not null")
}

func TestReadManifest_MissingOrCorrupt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, ok := ReadManifest(dir)
	assert.False(t, ok)

	require.NoError(t, os.WriteFile(ManifestPath(dir), []byte("{not json"), 0o600))
	_, ok = ReadManifest(dir)
	assert.False(t, ok)
}

// TestManifestSurvivesARoundTrip guards the wire shape ml-service reads.
func TestManifestSurvivesARoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeManifest(dir, []string{"T20I"}, true, Provenance{DatasetSHA256: "abc", DatasetFeed: "t20s"})

	raw, err := os.ReadFile(ManifestPath(dir))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	provenance, ok := decoded["provenance"].(map[string]any)
	require.True(t, ok, "ml-service reads manifest.provenance by that name")
	assert.Equal(t, "abc", provenance["dataset_sha256"])
	assert.Equal(t, "t20s", provenance["dataset_feed"])
}

func TestProvenanceKnown(t *testing.T) {
	t.Parallel()
	assert.False(t, Provenance{}.Known())
	assert.False(t, Provenance{DatasetFeed: "all"}.Known(), "a feed without a digest identifies nothing")
	assert.True(t, Provenance{DatasetSHA256: "abc"}.Known())
}

func TestDescribeManifest(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "dataset unknown", describeManifest(Manifest{}))
	assert.Contains(t, describeManifest(Manifest{Provenance: Provenance{DatasetSHA256: "abc"}}), "abc")
}
