package server

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

func battingStep(t *testing.T) pipelinesvc.Step {
	t.Helper()
	step, ok := pipelinesvc.Steps().ByID("train_batting")
	require.True(t, ok)
	return step
}

// writeManifest puts a dataset manifest in a temp dataset directory, as an extract
// would have left behind.
func writeManifest(t *testing.T, manifest dataacquire.ExtractResult) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(dataset.DirEnvVar, dir)
	encoded, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dataacquire.ManifestPath(dir), encoded, 0o600))
}

// TestTrainRunMetadata_ClosesTheProvenanceLoop is what O-4 is for: the run records
// which dataset produced it, so "which data produced this model?" is a lookup rather
// than an archaeology exercise.
func TestTrainRunMetadata_ClosesTheProvenanceLoop(t *testing.T) {
	writeManifest(t, dataacquire.ExtractResult{
		ArchiveSHA256: "deadbeef",
		SourceURL:     "https://cricsheet.org/downloads/all_json.zip",
		FeedID:        "all",
		ExtractedAt:   "2026-08-26T12:00:00Z",
		MatchFiles:    19998,
	})

	meta := trainRunMetadata(battingStep(t), "2026-01-01T00:00:00Z", &pipelinesvc.TrainResult{
		Status:  "ok",
		Step:    "batting",
		Summary: map[string]interface{}{"formats_completed": float64(4)},
	})

	assert.Equal(t, "train_batting", meta["step"])
	assert.Equal(t, "2026-01-01T00:00:00Z", meta["cutoff"])

	summary, ok := meta["summary"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(4), summary["formats_completed"])

	provenance, ok := meta["provenance"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "deadbeef", provenance["dataset_sha256"])
	assert.Equal(t, "all", provenance["dataset_feed"])
	assert.Equal(t, 19998, provenance["dataset_match_files"])
	assert.Contains(t, provenance["dataset_source_url"], "cricsheet.org")
}

// TestTrainRunMetadata_OmitsProvenanceItCannotEstablish: a dataset directory
// populated by hand has no manifest, and inventing a digest would be worse than
// recording none — P-2 exists to flag exactly that case.
func TestTrainRunMetadata_OmitsProvenanceItCannotEstablish(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(dataset.DirEnvVar, dir)

	meta := trainRunMetadata(battingStep(t), "2026-01-01T00:00:00Z", nil)

	assert.Equal(t, "train_batting", meta["step"])
	assert.NotContains(t, meta, "provenance")
	assert.NotContains(t, meta, "summary")
}

// TestTrainRunMetadata_PartialManifestRecordsWhatItHas: an archive placed by hand and
// extracted has a digest but no feed or URL. Unknown is omitted, not blanked.
func TestTrainRunMetadata_PartialManifestRecordsWhatItHas(t *testing.T) {
	writeManifest(t, dataacquire.ExtractResult{ArchiveSHA256: "abc123", MatchFiles: 12})

	meta := trainRunMetadata(battingStep(t), "", nil)

	provenance, ok := meta["provenance"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "abc123", provenance["dataset_sha256"])
	assert.NotContains(t, provenance, "dataset_feed")
	assert.NotContains(t, provenance, "dataset_source_url")
}

// TestTrainRunMetadata_SurvivesAnUninstrumentedStep: combination_meta publishes no
// progress, so its result carries no summary. The run is still worth recording.
func TestTrainRunMetadata_SurvivesAnUninstrumentedStep(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(dataset.DirEnvVar, dir)

	meta := trainRunMetadata(battingStep(t), "cutoff", &pipelinesvc.TrainResult{Status: "ok", Step: "batting"})

	assert.Equal(t, "train_batting", meta["step"])
	assert.NotContains(t, meta, "summary", "an empty summary is not recorded as an empty object")
}

// TestTrainRunMetadata_IsJSONSerialisable: it goes straight into a jsonb column, and
// a value that cannot marshal would be dropped at the last moment with only a log.
func TestTrainRunMetadata_IsJSONSerialisable(t *testing.T) {
	writeManifest(t, dataacquire.ExtractResult{ArchiveSHA256: "abc", FeedID: "t20s"})

	meta := trainRunMetadata(battingStep(t), "cutoff", &pipelinesvc.TrainResult{
		Summary: map[string]interface{}{
			"formats": []interface{}{map[string]interface{}{"format": "T20I", "rows": float64(1200)}},
		},
	})

	encoded, err := json.Marshal(meta)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"dataset_sha256":"abc"`)
	assert.Contains(t, string(encoded), `"format":"T20I"`)
}

// TestLiveDatasetProvenance_ReadsTheDirectoryNotACache: the dataset directory can be
// replaced by an extract between one export and the next, and a cached digest would
// then describe data that is no longer there. A provenance record that is quietly
// wrong is worse than none — the whole reason P-2 exists.
func TestLiveDatasetProvenance_ReadsTheDirectoryNotACache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(dataset.DirEnvVar, dir)

	assert.False(t, liveDatasetProvenance().Known(), "an empty directory knows nothing")

	first, err := json.Marshal(dataacquire.ExtractResult{ArchiveSHA256: "first", FeedID: "t20s"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dataacquire.ManifestPath(dir), first, 0o600))
	assert.Equal(t, "first", liveDatasetProvenance().DatasetSHA256)

	// A later extract replaces the dataset; the next export must say so.
	second, err := json.Marshal(dataacquire.ExtractResult{ArchiveSHA256: "second", FeedID: "all"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dataacquire.ManifestPath(dir), second, 0o600))
	assert.Equal(t, "second", liveDatasetProvenance().DatasetSHA256)
	assert.Equal(t, "all", liveDatasetProvenance().DatasetFeed)
}

func TestLiveDatasetProvenance_CarriesEveryFieldTheManifestHas(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(dataset.DirEnvVar, dir)
	encoded, err := json.Marshal(dataacquire.ExtractResult{
		ArchiveSHA256: "abc",
		SourceURL:     "https://cricsheet.org/downloads/all_json.zip",
		FeedID:        "all",
		ExtractedAt:   "2026-08-26T10:00:00Z",
		MatchFiles:    19998,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dataacquire.ManifestPath(dir), encoded, 0o600))

	got := liveDatasetProvenance()
	assert.Equal(t, "abc", got.DatasetSHA256)
	assert.Contains(t, got.DatasetSourceURL, "cricsheet.org")
	assert.Equal(t, "all", got.DatasetFeed)
	assert.Equal(t, "2026-08-26T10:00:00Z", got.DatasetExtracted)
	assert.Equal(t, 19998, got.DatasetMatchFile)
}

// TestEveryExportPathStampsProvenance guards the gap this nearly shipped with: the
// run-plan executor builds its own export options (stepJob), and a manually triggered
// export builds another (runExportHandler). One carried provenance and the other did
// not, so a plan-driven export would have written a manifest naming no dataset —
// silently, which is the failure this phase exists to prevent.
//
// A source check because the two call sites are what must agree, and constructing a
// real export needs a database.
func TestEveryExportPathStampsProvenance(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"step_work.go", "pipeline_handlers.go"} {
		source, err := os.ReadFile(path)
		require.NoError(t, err)

		text := string(source)
		if !strings.Contains(text, "exportsvc.Options{") {
			continue
		}
		assert.Contains(t, text, "liveDatasetProvenance()",
			"%s builds export options without provenance; its manifest would name no dataset", path)
	}
}
