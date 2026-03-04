package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStepToCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		step string
		want string
	}{
		{name: "train_batting", step: "train_batting", want: "make train-batting CUTOFF=2025-01-01T00:00:00Z"},
		{name: "train_bowling", step: "train_bowling", want: "make train-bowling CUTOFF=2025-01-01T00:00:00Z"},
		{name: "train_fielding", step: "train_fielding", want: "make train-fielding CUTOFF=2025-01-01T00:00:00Z"},
		{name: "train_extras", step: "train_extras", want: "make train-extras CUTOFF=2025-01-01T00:00:00Z"},
		{name: "train_win", step: "train_win", want: "make train-win CUTOFF=2025-01-01T00:00:00Z"},
		{name: "train_innings", step: "train_innings", want: "make train-innings CUTOFF=2025-01-01T00:00:00Z"},
		{name: "auto_tune", step: "auto_tune", want: "make ml-auto-tune MODEL=all ALL_FORMATS=1"},
		{name: "train_combination_meta", step: "train_combination_meta", want: "make train-combination-meta CSV=<export_dir>/backtest_contributions.csv OUT=<export_dir>/combination_meta.json"},
		{name: "unknown_returns_empty", step: "unknown_step", want: ""},
		{name: "empty_returns_empty", step: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, StepToCommand(tt.step))
		})
	}
}

func TestTrainingStepToModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, TrainingStepToModel(tt.stepID))
		})
	}
}

func TestMLServiceBaseURL(t *testing.T) {
	tests := []struct {
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ML_SERVICE_URL", tt.envVal)
			got := MLServiceBaseURL()
			assert.Contains(t, got, tt.wantHas)
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
