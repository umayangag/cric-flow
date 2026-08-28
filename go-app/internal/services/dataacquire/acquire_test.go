package dataacquire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acquireSource = "https://cricsheet.org/downloads/all_json.zip"

// writeManifestFor puts a manifest in dataDir naming the given source.
func writeManifestFor(t *testing.T, dataDir, sourceURL string) {
	t.Helper()
	data, err := json.Marshal(ExtractResult{
		ArchivePath:   filepath.Join("staging", "all_json.zip"),
		ArchiveSHA256: "abc123",
		SourceURL:     sourceURL,
		DestDir:       dataDir,
		MatchFiles:    12,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(ManifestPath(dataDir), data, 0o600))
}

// stageArchive writes an archive in stagingDir with a sidecar naming its source.
func stageArchiveFromSource(t *testing.T, stagingDir, name, sourceURL string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(stagingDir, 0o755))
	archive := filepath.Join(stagingDir, name)
	require.NoError(t, os.WriteFile(archive, []byte("zip"), 0o600))
	writeSidecar(archive, Result{Path: archive, SourceURL: sourceURL})
}

func TestPlanAcquisition(t *testing.T) {
	t.Run("nothing on the box means fetch and extract both run", func(t *testing.T) {
		dirs := t.TempDir()
		got := PlanAcquisition(acquireSource, filepath.Join(dirs, "_staging"), dirs, 0, false)
		assert.Empty(t, got.SkipFetch)
		assert.Empty(t, got.SkipExtract)
	})

	t.Run("a live dataset from this source skips both, and says why", func(t *testing.T) {
		dataDir := t.TempDir()
		writeManifestFor(t, dataDir, acquireSource)
		got := PlanAcquisition(acquireSource, filepath.Join(dataDir, "_staging"), dataDir, 12, false)
		assert.Contains(t, got.SkipFetch, "already holds")
		assert.Contains(t, got.SkipFetch, "12 match files")
		assert.Equal(t, got.SkipFetch, got.SkipExtract)
	})

	t.Run("a manifest beside an empty directory is not evidence", func(t *testing.T) {
		// Someone deleted the match files. The manifest still describes what *was*
		// there, and trusting it would import nothing and report success.
		dataDir := t.TempDir()
		writeManifestFor(t, dataDir, acquireSource)
		got := PlanAcquisition(acquireSource, filepath.Join(dataDir, "_staging"), dataDir, 0, false)
		assert.Empty(t, got.SkipFetch)
	})

	t.Run("a live dataset from a different source is re-acquired", func(t *testing.T) {
		dataDir := t.TempDir()
		writeManifestFor(t, dataDir, "https://cricsheet.org/downloads/t20s_json.zip")
		got := PlanAcquisition(acquireSource, filepath.Join(dataDir, "_staging"), dataDir, 12, false)
		assert.Empty(t, got.SkipFetch)
		assert.Empty(t, got.SkipExtract)
	})

	t.Run("an already-staged archive skips the download but not the extract", func(t *testing.T) {
		dataDir := t.TempDir()
		staging := filepath.Join(dataDir, "_staging")
		stageArchiveFromSource(t, staging, "all_json.zip", acquireSource)
		got := PlanAcquisition(acquireSource, staging, dataDir, 0, false)
		assert.Contains(t, got.SkipFetch, "already staged")
		assert.Empty(t, got.SkipExtract)
	})

	t.Run("an archive with no recorded source is not assumed to be ours", func(t *testing.T) {
		dataDir := t.TempDir()
		staging := filepath.Join(dataDir, "_staging")
		require.NoError(t, os.MkdirAll(staging, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(staging, "mystery.zip"), []byte("zip"), 0o600))
		got := PlanAcquisition(acquireSource, staging, dataDir, 0, false)
		assert.Empty(t, got.SkipFetch, "skipping a download on no evidence is how stale data survives")
	})

	t.Run("refresh overrides every skip", func(t *testing.T) {
		dataDir := t.TempDir()
		writeManifestFor(t, dataDir, acquireSource)
		got := PlanAcquisition(acquireSource, filepath.Join(dataDir, "_staging"), dataDir, 12, true)
		assert.Empty(t, got.SkipFetch)
		assert.Empty(t, got.SkipExtract)
	})
}
