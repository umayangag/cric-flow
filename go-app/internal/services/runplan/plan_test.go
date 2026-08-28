package runplan

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

func ids(steps []pipelinesvc.Step) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.ID)
	}
	return out
}

// TestPlansAreDerivedFromTheRegistry: a hardcoded step list here would be the seventh
// copy of the pipeline, and the one that silently omits the next step someone adds.
func TestPlansAreDerivedFromTheRegistry(t *testing.T) {
	t.Parallel()
	full, err := Describe(PlanFull)
	require.NoError(t, err)

	var expected []string
	for _, step := range pipelinesvc.Steps().OnSurface(pipelinesvc.SurfacePipeline) {
		if !step.Optional {
			expected = append(expected, step.ID)
		}
	}
	assert.Equal(t, expected, ids(full))
}

// TestFullExcludesOptionalSteps: a "run everything" that silently included auto-tune
// would take hours nobody asked for. Optional means offered, never implied.
func TestFullExcludesOptionalSteps(t *testing.T) {
	t.Parallel()
	full, err := Describe(PlanFull)
	require.NoError(t, err)

	assert.NotContains(t, ids(full), "auto_tune")
	assert.NotContains(t, ids(full), "train_combination_meta")
	assert.Contains(t, ids(full), "import")
	assert.Contains(t, ids(full), "train_batting")
}

func TestRetrainOnlySkipsTheDataSteps(t *testing.T) {
	t.Parallel()
	plan, err := Describe(PlanRetrainOnly)
	require.NoError(t, err)

	assert.NotContains(t, ids(plan), "import")
	assert.NotContains(t, ids(plan), "precompute")
	assert.NotContains(t, ids(plan), "export")
	assert.Contains(t, ids(plan), "train_batting")
}

func TestDataRefreshSkipsTheModels(t *testing.T) {
	t.Parallel()
	plan, err := Describe(PlanDataRefresh)
	require.NoError(t, err)

	assert.Equal(t, []string{"import", "precompute", "export"}, ids(plan))
}

func TestUnknownPlanNamesTheOnesThatExist(t *testing.T) {
	t.Parallel()
	_, err := Describe("everything")
	require.Error(t, err)
	for _, name := range Names() {
		assert.Contains(t, err.Error(), name)
	}
}

// TestResolveReordersAnExplicitList: the registry's order is the dependency order.
// Running a caller's arbitrary sequence would mean export before precompute because
// someone typed it that way — which the gate would then refuse one step in, having
// already run the others.
func TestResolveReordersAnExplicitList(t *testing.T) {
	t.Parallel()
	steps, err := Resolve("", []string{"export", "import", "precompute"})
	require.NoError(t, err)
	assert.Equal(t, []string{"import", "precompute", "export"}, ids(steps))
}

func TestResolveRejectsAmbiguityRatherThanPickingOne(t *testing.T) {
	t.Parallel()
	_, err := Resolve(PlanFull, []string{"import"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not both")
}

func TestResolveRejectsUnknownAndNonPipelineSteps(t *testing.T) {
	t.Parallel()
	_, err := Resolve("", []string{"nonsense"})
	assert.Error(t, err)

	// fetch and extract are registry steps, but they are not stages of the pipeline.
	_, err = Resolve("", []string{"fetch"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dataset step")

	_, err = Resolve("", nil)
	assert.Error(t, err, "an empty plan is a mistake, not a no-op")
}

func TestResolveDeduplicates(t *testing.T) {
	t.Parallel()
	steps, err := Resolve("", []string{"import", "import", "precompute"})
	require.NoError(t, err)
	assert.Equal(t, []string{"import", "precompute"}, ids(steps))
}

func TestFirstIncompleteFindsWhereToResume(t *testing.T) {
	t.Parallel()
	state := State{Steps: []StepState{
		{StepID: "import", Status: StatusCompleted},
		{StepID: "precompute", Status: StatusFailed},
		{StepID: "export", Status: StatusPending},
	}}

	next, ok := state.FirstIncomplete()
	require.True(t, ok)
	assert.Equal(t, "precompute", next.StepID, "resume from the failure, not from the top")
}

func TestFirstIncompleteSkipsSkipped(t *testing.T) {
	t.Parallel()
	state := State{Steps: []StepState{
		{StepID: "import", Status: StatusSkipped},
		{StepID: "precompute", Status: StatusPending},
	}}

	next, ok := state.FirstIncomplete()
	require.True(t, ok)
	assert.Equal(t, "precompute", next.StepID)
}

func TestDoneRequiresEveryStepTerminal(t *testing.T) {
	t.Parallel()
	running := State{Steps: []StepState{
		{Status: StatusCompleted}, {Status: StatusRunning},
	}}
	assert.False(t, running.Done())

	stopped := State{Steps: []StepState{
		{Status: StatusCompleted}, {Status: StatusFailed}, {Status: StatusPending},
	}}
	assert.False(t, stopped.Done(), "a plan stopped with steps still pending is not done")

	finished := State{Steps: []StepState{{Status: StatusCompleted}, {Status: StatusSkipped}}}
	assert.True(t, finished.Done())
}

// TestCloneIsDeep guards the Store contract: an implementation that retains what it
// was handed must not watch it change underneath.
func TestCloneIsDeep(t *testing.T) {
	t.Parallel()
	original := State{Plan: PlanFull, Steps: []StepState{{StepID: "import", Status: StatusPending}}}
	snapshot := original.Clone()

	original.Steps[0].Status = StatusCompleted

	assert.Equal(t, StatusPending, snapshot.Steps[0].Status)
	assert.Equal(t, StatusCompleted, original.Steps[0].Status)
}

func TestStateSurvivesAJSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := NewState(PlanFull, mustResolve(t, "import", "precompute"), "2026-08-27T12:00:00Z")
	original.Steps[0].Status = StatusCompleted

	encoded, err := json.Marshal(original)
	require.NoError(t, err)

	restored, ok := StateFromJSON(encoded)
	require.True(t, ok)
	assert.Equal(t, original, restored)
}

func TestStateFromJSONRejectsWhatIsNotAPlan(t *testing.T) {
	t.Parallel()
	_, ok := StateFromJSON(nil)
	assert.False(t, ok)

	_, ok = StateFromJSON(json.RawMessage(`{"files": 12, "dir": "/data"}`))
	assert.False(t, ok, "an import's metadata is not a plan state")

	_, ok = StateFromJSON(json.RawMessage(`{not json`))
	assert.False(t, ok)
}

func mustResolve(t *testing.T, ids ...string) []pipelinesvc.Step {
	t.Helper()
	steps, err := Resolve("", ids)
	require.NoError(t, err)
	return steps
}

// TestImportPlanAcquiresBeforeImporting pins the order consumer plan W6-2 depends on.
//
// It is the only place the dependency "import needs data on disk" is written down:
// fetch and extract are on SurfaceData and carry no Requires, so no predicate over
// the pipeline surface can produce this sequence, and nothing else would notice if it
// were reordered into "import, then download the data it just imported".
func TestImportPlanAcquiresBeforeImporting(t *testing.T) {
	t.Parallel()

	steps, err := Resolve(PlanImport, nil)
	require.NoError(t, err)

	ids := make([]string, 0, len(steps))
	for _, step := range steps {
		ids = append(ids, step.ID)
	}
	require.Equal(t, []string{"fetch", "extract", "import"}, ids)
}

// TestImportPlanIsOffered keeps the plan discoverable: a plan the API will run but
// never names is one an operator finds by guessing.
func TestImportPlanIsOffered(t *testing.T) {
	t.Parallel()
	require.Contains(t, Names(), PlanImport)
}
