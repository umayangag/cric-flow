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

// TestFullExcludesOptionalSteps: a "run everything" that silently included evaluate
// would spend the whole harness on a run nobody asked to measure. Optional means
// offered, never implied.
func TestFullExcludesOptionalSteps(t *testing.T) {
	t.Parallel()
	full, err := Describe(PlanFull)
	require.NoError(t, err)

	assert.NotContains(t, ids(full), "evaluate")
	assert.Equal(t, []string{"import", "retrain", "reload"}, ids(full))
}

// TestRetrainOnlySkipsTheImport: the data is already in the database; what is stale is
// the model. It still ends in a reload, because a run nothing points at is not serving.
func TestRetrainOnlySkipsTheImport(t *testing.T) {
	t.Parallel()
	plan, err := Describe(PlanRetrainOnly)
	require.NoError(t, err)

	assert.Equal(t, []string{"retrain", "reload"}, ids(plan))
}

// TestRefreshIsAcquisitionThenTraining: the scheduled cadence (A-5) is one walk from
// the archive to the run being served. Acquisition and training being two plans is
// what let them come apart — an import with no retrain after it left the served
// ratings nine days old against a fourteen-day limit.
func TestRefreshIsAcquisitionThenTraining(t *testing.T) {
	t.Parallel()
	plan, err := Describe(PlanRefresh)
	require.NoError(t, err)

	assert.Equal(t, []string{"fetch", "extract", "import", "retrain", "reload"}, ids(plan))
}

// TestRefreshIsComposedOfTheTwoPlansItChains: written out, its step list would be the
// copy that keeps the old shape after the registry changes.
func TestRefreshIsComposedOfTheTwoPlansItChains(t *testing.T) {
	t.Parallel()
	acquire, err := Describe(PlanImport)
	require.NoError(t, err)
	train, err := Describe(PlanRetrainOnly)
	require.NoError(t, err)
	refresh, err := Describe(PlanRefresh)
	require.NoError(t, err)

	assert.Equal(t, append(ids(acquire), ids(train)...), ids(refresh))
}

// TestRefreshEndsInReloadAndExcludesEvaluate: publishing is the last step, so a
// failure anywhere before it leaves `current` where it was; and the 54-minute harness
// is never implied by a cadence nobody asked to measure.
func TestRefreshEndsInReloadAndExcludesEvaluate(t *testing.T) {
	t.Parallel()
	plan, err := Describe(PlanRefresh)
	require.NoError(t, err)

	assert.Equal(t, "reload", ids(plan)[len(plan)-1])
	assert.NotContains(t, ids(plan), "evaluate")
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
// Running a caller's arbitrary sequence would mean reload before retrain because
// someone typed it that way — which the gate would then refuse one step in, having
// already run the others.
func TestResolveReordersAnExplicitList(t *testing.T) {
	t.Parallel()
	steps, err := Resolve("", []string{"reload", "import", "retrain"})
	require.NoError(t, err)
	assert.Equal(t, []string{"import", "retrain", "reload"}, ids(steps))
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
	steps, err := Resolve("", []string{"import", "import", "retrain"})
	require.NoError(t, err)
	assert.Equal(t, []string{"import", "retrain"}, ids(steps))
}

func TestFirstIncompleteFindsWhereToResume(t *testing.T) {
	t.Parallel()
	state := State{Steps: []StepState{
		{StepID: "import", Status: StatusCompleted},
		{StepID: "retrain", Status: StatusFailed},
		{StepID: "reload", Status: StatusPending},
	}}

	next, ok := state.FirstIncomplete()
	require.True(t, ok)
	assert.Equal(t, "retrain", next.StepID, "resume from the failure, not from the top")
}

func TestFirstIncompleteSkipsSkipped(t *testing.T) {
	t.Parallel()
	state := State{Steps: []StepState{
		{StepID: "import", Status: StatusSkipped},
		{StepID: "retrain", Status: StatusPending},
	}}

	next, ok := state.FirstIncomplete()
	require.True(t, ok)
	assert.Equal(t, "retrain", next.StepID)
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
	original := NewState(PlanFull, mustResolve(t, "import", "retrain"), "2026-08-27T12:00:00Z")
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
