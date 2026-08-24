package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTrainingStepToModel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		stepID string
		want   string
	}{
		{name: "batting", stepID: "train_batting", want: "batting"},
		{name: "bowling", stepID: "train_bowling", want: "bowling"},
		{name: "fielding", stepID: "train_fielding", want: "fielding"},
		{name: "extras", stepID: "train_extras", want: "extras"},
		{name: "innings", stepID: "train_innings", want: "innings"},
		{name: "win", stepID: "train_win", want: "win"},
		{name: "unknown_returns_empty", stepID: "auto_tune", want: ""},
		{name: "empty_returns_empty", stepID: "", want: ""},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, TrainingStepToModel(tc.stepID))
		})
	}
}

func TestMLServiceBaseURL(t *testing.T) {
	testCases := []struct {
		name    string
		envVal  string
		wantHas string // substring the result must contain
	}{
		{
			name:    "env_override",
			envVal:  "http://ml:9000/",
			wantHas: "http://ml:9000",
		},
		{
			name:    "env_override_no_trailing_slash",
			envVal:  "http://ml:9000",
			wantHas: "http://ml:9000",
		},
		{
			name:    "empty_env_uses_fallback",
			envVal:  "",
			wantHas: "http", // fallback always contains http
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ML_SERVICE_URL", tc.envVal)
			got := MLServiceBaseURL()
			assert.Contains(t, got, tc.wantHas)
			// Must never end with trailing slash.
			assert.NotRegexp(t, `/$`, got)
		})
	}
}

func TestDefaultCutoff(t *testing.T) {
	t.Parallel()

	before := time.Now().UTC()
	cutoff := DefaultCutoff()
	after := time.Now().UTC()

	parsed, err := time.Parse(time.RFC3339, cutoff)
	assert.NoError(t, err)
	assert.False(t, parsed.Before(before.Truncate(time.Second)), "cutoff should not be before test start")
	assert.False(t, parsed.After(after.Add(time.Second)), "cutoff should not be after test end")
}
