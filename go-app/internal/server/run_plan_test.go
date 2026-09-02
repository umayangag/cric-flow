package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/services/runplan"
)

// TestPlanCancel_StopsThePlanNotJustTheStep is R-1's cancellation requirement.
// Cancelling only the step would stop that step and then let the plan start the next
// one, which is not what Stop means.
func TestPlanCancel_StopsThePlanNotJustTheStep(t *testing.T) {
	t.Parallel()
	app := &App{}

	planCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.planCancel.set(cancel)

	assert.True(t, app.StopRunPlan())
	assert.Error(t, planCtx.Err(), "the plan's own context must be cancelled")
}

func TestStopRunPlan_ReportsWhenThereIsNoPlan(t *testing.T) {
	t.Parallel()
	app := &App{}
	assert.False(t, app.StopRunPlan(), "nothing to stop is not the same as stopped")

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.planCancel.set(cancel)
	assert.True(t, app.StopRunPlan())
	assert.False(t, app.StopRunPlan(), "a second Stop has nothing left")
}

// TestPlanCommandIsNotARegistryStep: were it one, the plan's own in-progress row would
// make LaneBusy true and block the very steps it exists to run.
func TestPlanCommandIsNotARegistryStep(t *testing.T) {
	t.Parallel()
	registry := pipelinesvc.Steps()

	_, known := registry.ByCommand(runplan.PlanCommand)
	assert.False(t, known, "a plan is not a stage of the pipeline")
	assert.NotContains(t, registry.CommandsInLane(pipelinesvc.LaneCompute), runplan.PlanCommand)
	assert.NotContains(t, registry.CommandsInLane(pipelinesvc.LaneData), runplan.PlanCommand)
}

// TestStepJob_IsTheSameWorkForEveryCaller: the executor and the single-step handlers
// share one definition of what a step does, so a plan cannot drift into running
// something different from a manual trigger.
func TestStepJob_IsTheSameWorkForEveryCaller(t *testing.T) {
	t.Parallel()
	app := &App{}

	for _, step := range pipelinesvc.Steps().OnSurface(pipelinesvc.SurfacePipeline) {
		job := app.stepJob(step, StepRequest{})
		assert.Equal(t, step.Command, job.Command, "%s records itself under its registry command", step.ID)
		assert.NotNil(t, job.Run, "%s has no work to do", step.ID)
		assert.Positive(t, job.Timeout, "%s has no timeout", step.ID)
		assert.NotNil(t, job.Args, "%s records no args", step.ID)
	}
}

// TestStepJob_ReloadCarriesTheRunItWasAskedFor: reload is the step that decides which
// run serves, so the run id has to survive being run by a plan rather than a handler --
// and a reload with no run id is a legitimate request (reload what `current` names).
func TestStepJob_ReloadCarriesTheRunItWasAskedFor(t *testing.T) {
	t.Parallel()
	app := &App{}
	step, ok := pipelinesvc.Steps().ByID("reload")
	require.True(t, ok)

	named := app.stepJob(step, StepRequest{RunID: "20260902T101500Z-ab12cd34"})
	assert.Equal(t, "20260902T101500Z-ab12cd34", named.Args["run_id"])

	unnamed := app.stepJob(step, StepRequest{})
	assert.NotContains(t, unnamed.Args, "run_id",
		"an unnamed reload must not claim a run it was not given")
}

func TestStepJob_DefaultsTheCutoff(t *testing.T) {
	t.Parallel()
	app := &App{}
	step, ok := pipelinesvc.Steps().ByID("retrain")
	require.True(t, ok)

	job := app.stepJob(step, StepRequest{})
	assert.Equal(t, pipelinesvc.DefaultCutoff(), job.Args["cutoff"])

	explicit := app.stepJob(step, StepRequest{Cutoff: "2026-01-01T00:00:00Z"})
	assert.Equal(t, "2026-01-01T00:00:00Z", explicit.Args["cutoff"])
}

// --- API (R-2) ---

func postRunPlan(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ops/pipeline/run-plan", strings.NewReader(body))
	rec := httptest.NewRecorder()
	(&App{}).runPlanStartHandler(rec, req)
	return rec
}

func TestRunPlanStart_RejectsAnUnknownPlanAndNamesTheRealOnes(t *testing.T) {
	t.Parallel()
	rec := postRunPlan(t, `{"plan":"everything"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var body struct {
		Error string   `json:"error"`
		Plans []string `json:"plans"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body.Error, "unknown plan")
	assert.ElementsMatch(t, runplan.Names(), body.Plans, "a refusal must say what would be accepted")
}

func TestRunPlanStart_RejectsAmbiguityRatherThanPickingOne(t *testing.T) {
	t.Parallel()
	rec := postRunPlan(t, `{"plan":"full","steps":["import"]}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "not both")
}

func TestRunPlanStart_RejectsANonPipelineStep(t *testing.T) {
	t.Parallel()
	rec := postRunPlan(t, `{"steps":["fetch"]}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "dataset step")
}

// TestRunPlanStart_EmptyBodyMeansTheObviousThing: an operator posting nothing wants
// the pipeline, not an error about which plan they forgot to name.
func TestRunPlanStart_EmptyBodyDefaultsToFull(t *testing.T) {
	// Not parallel: it starts a background plan against the App's job context.
	db.SetDB(nil) // no database, so the executor stops at Create without running steps
	rec := postRunPlan(t, ``)

	require.Equal(t, http.StatusAccepted, rec.Code)
	var body struct {
		Plan  string   `json:"plan"`
		Steps []string `json:"steps"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, runplan.PlanFull, body.Plan)
	assert.Contains(t, body.Steps, "import")
	assert.NotContains(t, body.Steps, "evaluate", "optional steps are never implied")
}

func TestRunPlanState_SaysNothingHasRunRatherThanErroring(t *testing.T) {
	t.Parallel()
	db.SetDB(nil)

	rec := httptest.NewRecorder()
	(&App{}).runPlanStateHandler(rec, httptest.NewRequest(http.MethodGet, "/ops/pipeline/plan", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Running bool     `json:"running"`
		Plans   []string `json:"plans"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Running)
	assert.ElementsMatch(t, runplan.Names(), body.Plans)
}

func TestRunPlanStart_RefusesToResumeWithNothingToResume(t *testing.T) {
	// Not parallel: it reads the plan store.
	db.SetDB(nil)
	rec := postRunPlan(t, `{"plan":"full","resume":true}`)

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "no previous plan to resume")
}
