package pipeline

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	Comment  string         `json:"$comment"`
	Version  int            `json:"version"`
	Steps    []contractStep `json:"pipeline_steps"`
	Rejected []string       `json:"rejected_query_params"`
}

type contractStep struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Optional     bool     `json:"optional"`
	Requires     []string `json:"requires"`
	Prerequisite string   `json:"prerequisite,omitempty"`
}

// rejectedQueryParams mirrors go-app's removed-parameter list. It lives here rather
// than being imported from internal/server because the contract is about what the
// frontend may send, and a copy that a test compares is safer than an import cycle.
var rejectedQueryParams = []string{"use_unified_model", "model=unified"}

func buildContract() contractDoc {
	steps := make([]contractStep, 0, len(Steps().All()))
	for _, s := range Steps().All() {
		requires := s.Requires
		if requires == nil {
			requires = []string{}
		}
		steps = append(steps, contractStep{
			ID:           s.ID,
			Label:        s.Label,
			Optional:     s.Optional,
			Requires:     requires,
			Prerequisite: s.Prerequisite,
		})
	}
	return contractDoc{
		Comment: "Generated from go-app/internal/services/pipeline/registry.go. Do not edit by hand; " +
			"run: go test ./internal/services/pipeline -run TestPipelineContract -update",
		Version:  1,
		Steps:    steps,
		Rejected: rejectedQueryParams,
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

		for _, req := range step.Requires {
			assert.True(t, registry.Has(req), "step %s requires unknown step %s", step.ID, req)
		}
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

func TestStepLookups(t *testing.T) {
	t.Parallel()
	registry := Steps()

	step, ok := registry.ByID("train_combination_meta")
	require.True(t, ok)
	assert.Equal(t, "train-combination-meta", step.Command)
	assert.True(t, step.Optional)
	assert.NotEmpty(t, step.Prerequisite, "combination-meta's CSV precondition must be stated")
	assert.True(t, step.RunsOnMLService())
	assert.False(t, step.IsTraining(), "combination-meta has no tuned-params model")

	byCommand, ok := registry.ByCommand("train-combination-meta")
	require.True(t, ok)
	assert.Equal(t, step, byCommand)

	_, ok = registry.ByID("does_not_exist")
	assert.False(t, ok)
	assert.False(t, registry.Has("does_not_exist"))
	assert.Equal(t, "some-external-command", registry.LabelForCommand("some-external-command"))
}
