package pipeline

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readContract loads the generated contract as the far side reads it — off disk, not
// out of the constants this package would otherwise be checking against itself.
func readContract(t *testing.T) contractDoc {
	t.Helper()
	raw, err := os.ReadFile(contractPath)
	require.NoError(t, err, "contract file missing; run TestPipelineContract with -update")
	var doc contractDoc
	require.NoError(t, json.Unmarshal(raw, &doc))
	return doc
}

// TestDefaultCutoffMatchesTheContract is go-app's half of H-24 for the cutoff.
//
// This is the test D-9 needed and nobody wrote: the old one asserted DefaultCutoff()
// was valid RFC3339 — true, and irrelevant, because ml-service never accepted RFC3339.
// Asserting against the contract instead of against this side's own assumption is the
// whole of the rule.
func TestDefaultCutoffMatchesTheContract(t *testing.T) {
	t.Parallel()
	contract := readContract(t)
	pattern := regexp.MustCompile(contract.Cutoff.Pattern)

	assert.Regexp(t, pattern, DefaultCutoff(),
		"the cutoff go-app sends when the console's box is empty must be in the wire format")
	assert.Regexp(t, pattern, contract.Cutoff.Example,
		"the contract's own example must match the pattern it publishes")
	assert.Equal(t, CutoffPattern, contract.Cutoff.Pattern, "the contract is stale; regenerate it")
	assert.Equal(t, CutoffHint, contract.Cutoff.Hint, "the contract is stale; regenerate it")
}

func TestDefaultCutoffIsTodayInUTC(t *testing.T) {
	t.Parallel()

	before := time.Now().UTC()
	cutoff := DefaultCutoff()
	after := time.Now().UTC()

	parsed, err := time.Parse(time.DateOnly, cutoff)
	require.NoError(t, err)
	assert.False(t, parsed.Before(before.Truncate(24*time.Hour)), "cutoff should not predate the test")
	assert.False(t, parsed.After(after), "cutoff should not be in the future")
}

func TestIsValidCutoff(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "the wire format", value: "2025-09-01", want: true},
		{name: "the default", value: DefaultCutoff(), want: true},
		{name: "the RFC3339 timestamp D-9 sent", value: "2026-09-02T18:33:11Z", want: false},
		{name: "empty", value: "", want: false},
		{name: "a date with a trailing word", value: "2025-09-01 or so", want: false},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, IsValidCutoff(tc.value))
		})
	}
}

// TestMLCallsCoverEveryTrainingStep keeps the declared boundary exhaustive: a training
// step added to the registry is an ml-service endpoint the far side's contract test
// starts requiring, without anyone remembering to list it.
func TestMLCallsCoverEveryTrainingStep(t *testing.T) {
	t.Parallel()
	declared := map[string]MLCall{}
	for _, call := range MLCalls() {
		assert.NotEmpty(t, call.Method, "every declared call needs a method")
		assert.NotEmpty(t, call.Query, "a call with no query parameters need not be declared")
		declared[call.Path] = call
	}

	for _, step := range Steps().All() {
		if !step.RunsOnMLService() {
			continue
		}
		call, ok := declared[MLTrainPathPrefix+step.MLEndpoint]
		require.True(t, ok, "step %s runs on ml-service but its endpoint is not declared", step.ID)
		assert.Equal(t, []string{MLQueryCutoff}, call.Query)
	}

	assert.Contains(t, declared, MLReloadPath, "reload is an ml-service call too")
	assert.Contains(t, declared, MLProgressPath, "progress is polled on every SSE tick")
}

func TestContractDeclaresTheSameBoundaryAsTheCode(t *testing.T) {
	t.Parallel()
	contract := readContract(t)

	require.Len(t, contract.MLCalls, len(MLCalls()))
	for i, call := range MLCalls() {
		assert.Equal(t, call.Path, contract.MLCalls[i].Path)
		assert.Equal(t, call.Method, contract.MLCalls[i].Method)
		assert.Equal(t, call.Query, contract.MLCalls[i].Query)
	}
}
