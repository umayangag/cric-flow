package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataacquire"
	"github.com/umayangag/cric-flow/go-app/internal/services/dataset"
)

// liveDataset points the dataset directory at a temp dir holding a manifest.
func liveDataset(t *testing.T, digest string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(dataset.DirEnvVar, dir)
	if digest == "" {
		return
	}
	encoded, err := json.Marshal(dataacquire.ExtractResult{
		ArchiveSHA256: digest,
		FeedID:        "all",
		MatchFiles:    19998,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dataacquire.ManifestPath(dir), encoded, 0o600))
}

func modelWith(provenance map[string]any) map[string]any {
	m := map[string]any{"model_name": "Batting", "match_format": "T20I"}
	if provenance != nil {
		m["provenance"] = provenance
	}
	return m
}

// TestAttachLiveDataset_MarksStaleAndCurrent is P-2's core: ml-service can say what a
// model was trained on, but only go-app knows whether that is still on the box.
func TestAttachLiveDataset_MarksStaleAndCurrent(t *testing.T) {
	liveDataset(t, "current-digest")

	current := modelWith(map[string]any{"dataset_sha256": "current-digest"})
	stale := modelWith(map[string]any{"dataset_sha256": "old-digest"})
	payload := map[string]any{}

	attachLiveDataset(payload, []any{current, stale})

	assert.Equal(t, true, current["dataset_is_live"])
	assert.Equal(t, false, stale["dataset_is_live"])

	live, ok := payload["live_dataset"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "current-digest", live["dataset_sha256"])
	assert.Equal(t, "all", live["dataset_feed"])
}

// TestAttachLiveDataset_UnknownIsNotStale: a model trained before provenance existed
// is unaccounted for, not out of date. Marking it stale would flag every model on a
// box that has not retrained since — noise rather than a warning.
func TestAttachLiveDataset_UnknownIsNotStale(t *testing.T) {
	liveDataset(t, "current-digest")

	noProvenance := modelWith(nil)
	emptyDigest := modelWith(map[string]any{"dataset_feed": "all"})
	payload := map[string]any{}

	attachLiveDataset(payload, []any{noProvenance, emptyDigest})

	assert.NotContains(t, noProvenance, "dataset_is_live", "no verdict is a third state")
	assert.NotContains(t, emptyDigest, "dataset_is_live")
}

// TestAttachLiveDataset_NoLiveDatasetMakesNoClaim: with nothing to compare against,
// telling the operator their models are stale would be the wrong advice.
func TestAttachLiveDataset_NoLiveDatasetMakesNoClaim(t *testing.T) {
	liveDataset(t, "")

	model := modelWith(map[string]any{"dataset_sha256": "some-digest"})
	payload := map[string]any{}

	attachLiveDataset(payload, []any{model})

	assert.NotContains(t, payload, "live_dataset")
	assert.NotContains(t, model, "dataset_is_live")
}

func TestAttachLiveDataset_IgnoresMalformedEntries(t *testing.T) {
	liveDataset(t, "current-digest")

	payload := map[string]any{}
	assert.NotPanics(t, func() {
		attachLiveDataset(payload, []any{"not a model", nil, 42, map[string]any{"provenance": "not a map"}})
	})
	assert.Contains(t, payload, "live_dataset")
}

// TestEnrichModelStatsPayload_AttachesProvenanceWithoutADatabase: a model being stale
// is worth knowing whether or not Postgres is up.
func TestEnrichModelStatsPayload_AttachesProvenanceWithoutADatabase(t *testing.T) {
	liveDataset(t, "current-digest")

	model := modelWith(map[string]any{"dataset_sha256": "old-digest"})
	payload := map[string]any{"models": []any{model}}

	enrichModelStatsPayload(payload, httptest.NewRequest(http.MethodGet, "/api/ml/model-stats", nil))

	assert.Equal(t, false, model["dataset_is_live"])
	assert.Contains(t, payload, "live_dataset")
	assert.Contains(t, payload, "hierarchy")
}
