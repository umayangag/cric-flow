package server

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

func retrainStep(t *testing.T) pipelinesvc.Step {
	t.Helper()
	step, ok := pipelinesvc.Steps().ByID("retrain")
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

	meta := trainRunMetadata(retrainStep(t), "2026-01-01T00:00:00Z", &pipelinesvc.TrainResult{
		Status:  "ok",
		Step:    "retrain",
		Summary: map[string]interface{}{"formats_completed": float64(4)},
	})

	assert.Equal(t, "retrain", meta["step"])
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

	meta := trainRunMetadata(retrainStep(t), "2026-01-01T00:00:00Z", nil)

	assert.Equal(t, "retrain", meta["step"])
	assert.NotContains(t, meta, "provenance")
	assert.NotContains(t, meta, "summary")
}

// TestTrainRunMetadata_PartialManifestRecordsWhatItHas: an archive placed by hand and
// extracted has a digest but no feed or URL. Unknown is omitted, not blanked.
func TestTrainRunMetadata_PartialManifestRecordsWhatItHas(t *testing.T) {
	writeManifest(t, dataacquire.ExtractResult{ArchiveSHA256: "abc123", MatchFiles: 12})

	meta := trainRunMetadata(retrainStep(t), "", nil)

	provenance, ok := meta["provenance"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "abc123", provenance["dataset_sha256"])
	assert.NotContains(t, provenance, "dataset_feed")
	assert.NotContains(t, provenance, "dataset_source_url")
}

// TestTrainRunMetadata_SurvivesAnUninstrumentedStep: a step that publishes no
// progress, so its result carries no summary. The run is still worth recording.
func TestTrainRunMetadata_SurvivesAnUninstrumentedStep(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(dataset.DirEnvVar, dir)

	meta := trainRunMetadata(retrainStep(t), "cutoff", &pipelinesvc.TrainResult{Status: "ok", Step: "retrain"})

	assert.Equal(t, "retrain", meta["step"])
	assert.NotContains(t, meta, "summary", "an empty summary is not recorded as an empty object")
}

// TestTrainRunMetadata_IsJSONSerialisable: it goes straight into a jsonb column, and
// a value that cannot marshal would be dropped at the last moment with only a log.
func TestTrainRunMetadata_IsJSONSerialisable(t *testing.T) {
	writeManifest(t, dataacquire.ExtractResult{ArchiveSHA256: "abc", FeedID: "t20s"})

	meta := trainRunMetadata(retrainStep(t), "cutoff", &pipelinesvc.TrainResult{
		Summary: map[string]interface{}{
			"formats": []interface{}{map[string]interface{}{"format": "T20I", "rows": float64(1200)}},
		},
	})

	encoded, err := json.Marshal(meta)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"dataset_sha256":"abc"`)
	assert.Contains(t, string(encoded), `"format":"T20I"`)
}
