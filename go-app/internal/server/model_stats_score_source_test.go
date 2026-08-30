package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ml-service labels a single-train score "holdout". When a later auto-tune puts a
// cross-validated score in the DB, that row wins the value — so it must win the label
// too, or the table shows a CV number described as a holdout.
func TestEnrichWithDBMetrics_HoldoutLabelledModel_BecomesTuningCV(t *testing.T) {
	testCases := []struct {
		name            string
		existing        map[string]any
		metrics         string
		wantScoreSource any
		wantDisplay     string
	}{
		{
			name:            "holdout score is relabelled when the DB has tuned metrics",
			existing:        map[string]any{"score_source": "holdout", "accuracy_display": "MAE=6.50"},
			metrics:         `{"mae": 5.1, "rmse": 7.2}`,
			wantScoreSource: "tuning_cv",
			wantDisplay:     "MAE=5.10, RMSE=7.20",
		},
		{
			name:            "unlabelled model gains the tuned label",
			existing:        map[string]any{},
			metrics:         `{"accuracy_pct": 63.4}`,
			wantScoreSource: "tuning_cv",
			wantDisplay:     "63.4%",
		},
		{
			name:            "metrics with no displayable score leave the label alone",
			existing:        map[string]any{"score_source": "holdout"},
			metrics:         `{"holdout_rows": 200}`,
			wantScoreSource: "holdout",
			wantDisplay:     "",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			modelMap := tc.existing

			enrichWithDBMetrics(modelMap, json.RawMessage(tc.metrics))

			require.Equal(t, tc.wantScoreSource, modelMap["score_source"])
			if tc.wantDisplay != "" {
				assert.Equal(t, tc.wantDisplay, modelMap["accuracy_display"])
			}
		})
	}
}
