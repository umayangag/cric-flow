package pipeline

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/formats"
	"github.com/umayangag/cric-flow/go-app/internal/services/apiparams"
)

// updateContract regenerates contracts/ops-console.contract.json instead of asserting
// against it. Run: go test ./internal/services/pipeline -run TestPipelineContract -update
var updateContract = flag.Bool("update", false, "rewrite the generated contract file")

// contractPath is the repo-root contract both the backend and the frontend test against.
const contractPath = "../../../../contracts/ops-console.contract.json"

// contractDoc is the generated contract. It is the only thing the frontend is allowed
// to assume about the backend's pipeline surface, and it is generated rather than
// hand-written so it cannot describe a backend that does not exist.
type contractDoc struct {
	Comment string         `json:"$comment"`
	Version int            `json:"version"`
	Steps   []contractStep `json:"pipeline_steps"`
	// DataSteps are the acquisition steps. They are a separate list rather than more
	// entries in pipeline_steps because they are not stages of the pipeline and must
	// not appear on its graph — the split Step.Surface encodes.
	DataSteps []contractStep `json:"data_steps"`
	Lanes     []string       `json:"lanes"`
	// Rejected and RejectedBody are the retired request parameters, generated from
	// apiparams rather than typed here, so a parameter retired on the server fails the
	// UI's tests in the same commit.
	//
	// They are two lists because the two are checked differently: a query parameter's
	// name only appears where a request is built, so the frontend greps every source
	// for it; a body field's name is an ordinary word that also appears in responses,
	// so only the module that builds requests is checked.
	Rejected     []string `json:"rejected_query_params"`
	RejectedBody []string `json:"rejected_body_params"`
	// Cutoff is the wire format of the training cutoff. It is in the contract because
	// D-9 was a value in *this* field being formatted by go-app as RFC3339 and parsed
	// by ml-service as a date — two components each tested against their own copy of
	// the assumption. H-24: the format is declared once and asserted from both sides.
	Cutoff contractCutoff `json:"cutoff"`
	// MLCalls is the ml-service admin surface go-app calls. ml-service's contract test
	// asserts every path here is a route that accepts these query parameters, so a
	// renamed endpoint fails a test rather than a run.
	MLCalls []contractMLCall `json:"ml_service_calls"`
	// FormatCodes is the format vocabulary both services match on. go-app writes these
	// into match rows; ml-service switches on them when it reads them back.
	FormatCodes []string `json:"format_codes"`
	// StopResponseField is the field go-app reads out of ml-service's stop answer to
	// learn which training steps really stopped. H-24's audit recorded that go-app
	// parsed nothing out of ml-service's bodies, and said this would become an H-24
	// item the moment it started; D-10's fix is that moment.
	StopResponseField string `json:"stop_response_field"`
	// TeamGenders is the gender half of a team's identity. go-app writes it into
	// opposition and match rows and now accepts it on the prediction request; the
	// frontend's picker sends it; ml-service matches on the literal when it groups the
	// E7 context baselines. Three copies of one vocabulary is the D-9 shape (D-11).
	TeamGenders []string `json:"team_genders"`
}

// contractCutoff is the cutoff's declared format: the pattern a value must match, how
// it is spelled to an operator, and one value of it for the far side to parse.
type contractCutoff struct {
	Pattern string `json:"pattern"`
	Hint    string `json:"hint"`
	Example string `json:"example"`
}

// contractMLCall is one endpoint on the go-app -> ml-service boundary.
type contractMLCall struct {
	Method string   `json:"method"`
	Path   string   `json:"path"`
	Query  []string `json:"query"`
}

type contractStep struct {
	ID string `json:"id"`
	// Command is what a run of this step is recorded as in data_migrations. The
	// frontend needs it to recognise its own run history — an accuracy trend that
	// cannot tell a retrain from a precompute cannot say what changed (W5-2) — and
	// hand-typing it there is how the six step tables drifted before F-1.
	Command      string   `json:"command"`
	Label        string   `json:"label"`
	Optional     bool     `json:"optional"`
	Requires     []string `json:"requires"`
	Prerequisite string   `json:"prerequisite,omitempty"`
}

func contractSteps(surface Surface) []contractStep {
	source := Steps().OnSurface(surface)
	steps := make([]contractStep, 0, len(source))
	for _, s := range source {
		requires := s.Requires
		if requires == nil {
			requires = []string{}
		}
		steps = append(steps, contractStep{
			ID:           s.ID,
			Command:      s.Command,
			Label:        s.Label,
			Optional:     s.Optional,
			Requires:     requires,
			Prerequisite: s.Prerequisite,
		})
	}
	return steps
}

func contractMLCalls() []contractMLCall {
	calls := MLCalls()
	out := make([]contractMLCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, contractMLCall(call))
	}
	return out
}

func buildContract() contractDoc {
	return contractDoc{
		Comment: "Generated from go-app/internal/services/pipeline/registry.go. Do not edit by hand; " +
			"run: go test ./internal/services/pipeline -run TestPipelineContract -update",
		Version:      2,
		Steps:        contractSteps(SurfacePipeline),
		DataSteps:    contractSteps(SurfaceData),
		Lanes:        LaneNames(),
		Rejected:     apiparams.QueryNames(),
		RejectedBody: apiparams.BodyNames(),
		Cutoff: contractCutoff{
			Pattern: CutoffPattern,
			Hint:    CutoffHint,
			Example: CutoffExample,
		},
		MLCalls:           contractMLCalls(),
		FormatCodes:       formats.CanonicalCodes(),
		TeamGenders:       TeamGenders(),
		StopResponseField: StopResponseField,
	}
}

// TestPipelineContract keeps the generated contract in step with the registry.
//
// The frontend asserts against the same file, so backend and UI can only disagree by
// one of the two tests failing. This is the drift that let train_combination_meta ship
// without a UI entry and let the UI keep offering use_unified_model after the backend
// stopped accepting it — one file, checked from both sides, closes both.
func TestPipelineContract(t *testing.T) {
	want, err := json.MarshalIndent(buildContract(), "", "  ")
	require.NoError(t, err)
	want = append(want, '\n')

	if *updateContract {
		require.NoError(t, os.MkdirAll(filepath.Dir(contractPath), 0o755))
		require.NoError(t, os.WriteFile(contractPath, want, 0o600))
		t.Log("contract regenerated:", contractPath)
		return
	}

	got, err := os.ReadFile(contractPath)
	require.NoError(t, err, "contract file missing; run with -update")
	assert.Equal(t, string(want), string(got),
		"the pipeline registry and the generated contract disagree; run with -update and update the frontend")
}

func TestRegistryIsInternallyConsistent(t *testing.T) {
	t.Parallel()
	registry := Steps()

	seenID := map[string]bool{}
	seenCommand := map[string]bool{}
	for _, step := range registry.All() {
		assert.NotEmpty(t, step.ID, "every step needs an ID")
		assert.NotEmpty(t, step.Command, "step %s needs a data_migrations command", step.ID)
		assert.NotEmpty(t, step.Label, "step %s needs a label", step.ID)

		assert.False(t, seenID[step.ID], "duplicate step ID %s", step.ID)
		assert.False(t, seenCommand[step.Command], "duplicate command %s", step.Command)
		seenID[step.ID] = true
		seenCommand[step.Command] = true

		assert.True(t, IsKnownLane(step.EffectiveLane()), "step %s declares unknown lane %s", step.ID, step.Lane)

		for _, req := range step.Requires {
			assert.True(t, registry.Has(req), "step %s requires unknown step %s", step.ID, req)
		}
	}
}

// TestSurfacesPartitionTheRegistry keeps the two contract lists exhaustive. A step
// added with a surface nobody renders would be accepted by the backend and invisible
// everywhere — the exact shape of the train_combination_meta gap, one level up.
func TestSurfacesPartitionTheRegistry(t *testing.T) {
	t.Parallel()
	pipelineSteps := Steps().OnSurface(SurfacePipeline)
	dataSteps := Steps().OnSurface(SurfaceData)
	assert.Len(t, Steps().All(), len(pipelineSteps)+len(dataSteps),
		"every step must be on exactly one rendered surface")
}

// TestAcquisitionRunsInTheDataLane guards the decision F-2 recorded: acquisition does
// not share the training lock. Were fetch to fall back to LaneCompute, a download
// would block training for as long as it ran, which is the system the lanes exist to
// avoid — and nothing else would fail to say so.
func TestAcquisitionRunsInTheDataLane(t *testing.T) {
	t.Parallel()
	for _, step := range Steps().OnSurface(SurfaceData) {
		assert.Equal(t, LaneData, step.EffectiveLane(), "data step %s must run in the data lane", step.ID)
	}
	for _, step := range Steps().OnSurface(SurfacePipeline) {
		assert.Equal(t, LaneCompute, step.EffectiveLane(), "pipeline step %s must run in the compute lane", step.ID)
	}
}

// TestRequirementsPrecedeTheirDependents guards the ordering the UI renders and the
// run-plan executor will walk: a step may only depend on steps declared before it.
func TestRequirementsPrecedeTheirDependents(t *testing.T) {
	t.Parallel()
	position := map[string]int{}
	for i, step := range Steps().All() {
		position[step.ID] = i
	}
	for _, step := range Steps().All() {
		for _, req := range step.Requires {
			assert.Less(t, position[req], position[step.ID],
				"step %s requires %s, which is declared after it", step.ID, req)
		}
	}
}

// TestEvaluateDependsOnTheImportOnly pins the edge that makes an evaluation runnable
// without a retrain. Evaluate is L4 at a cutoff of the operator's choosing over rows
// already in the database; requiring the retrain would gate "what would this have
// scored?" behind producing the artifacts it is not measuring.
func TestEvaluateDependsOnTheImportOnly(t *testing.T) {
	t.Parallel()
	step, ok := Steps().ByID("evaluate")
	require.True(t, ok)
	assert.Equal(t, []string{"import"}, step.Requires)
	assert.True(t, step.Optional, "a harness run nobody asked for must never be implied")
}

func TestStepLookups(t *testing.T) {
	t.Parallel()
	registry := Steps()

	step, ok := registry.ByID("evaluate")
	require.True(t, ok)
	assert.Equal(t, "xi-evaluate", step.Command)
	assert.True(t, step.Optional)
	assert.True(t, step.RunsOnMLService())

	byCommand, ok := registry.ByCommand("xi-evaluate")
	require.True(t, ok)
	assert.Equal(t, step, byCommand)

	_, ok = registry.ByID("does_not_exist")
	assert.False(t, ok)
	assert.False(t, registry.Has("does_not_exist"))
	assert.Equal(t, "some-external-command", registry.LabelForCommand("some-external-command"))
}
