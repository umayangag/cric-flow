package opsstatus

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// scanBoolRow implements db.Row for CanRunPipelineStep tests.
type scanBoolRow bool

func (r scanBoolRow) Scan(dest ...any) error {
	if len(dest) < 1 {
		return nil
	}
	if p, ok := dest[0].(*bool); ok {
		*p = bool(r)
		return nil
	}
	return nil
}

func setupPipelineDB(t *testing.T, mockDB *mocks.MockDB) {
	t.Helper()
	db.SetDB(mockDB)
	t.Cleanup(func() { db.SetDB(nil) })
}

func TestCanRunPipelineStep(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	cases := []struct {
		name    string
		setup   func(*mocks.MockDB)
		stepID  string
		wantOk  bool
		wantMsg string
	}{
		{
			name:    "unknown_step_returns_false",
			setup:   func(*mocks.MockDB) { db.SetDB(nil) },
			stepID:  "unknown_step",
			wantOk:  false,
			wantMsg: "unknown step",
		},
		{
			name: "another_running_returns_false",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanBoolRow(true))
			},
			stepID:  "import",
			wantOk:  false,
			wantMsg: "this step is already running",
		},
		{
			name: "import_no_previous_runnable",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanBoolRow(false))
			},
			stepID:  "import",
			wantOk:  true,
			wantMsg: "",
		},
		{
			name: "precompute_prev_done_runnable",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "precompute-features" && arr[1] == tracking.StatusInProgress
				})).Return(scanBoolRow(false))
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "cricsheet-import" && arr[1] == tracking.StatusCompleted
				})).Return(scanBoolRow(true))
			},
			stepID:  "precompute",
			wantOk:  true,
			wantMsg: "",
		},
		{
			name: "precompute_prev_not_done_not_runnable",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "precompute-features" && arr[1] == tracking.StatusInProgress
				})).Return(scanBoolRow(false))
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "cricsheet-import" && arr[1] == tracking.StatusCompleted
				})).Return(scanBoolRow(false))
			},
			stepID:  "precompute",
			wantOk:  false,
			wantMsg: "complete the previous step (Import) first",
		},
		{
			name: "auto_tune_allowed_when_no_one_running",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "ml-auto-tune" && arr[1] == tracking.StatusInProgress
				})).Return(scanBoolRow(false))
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "train-fielding" && arr[1] == tracking.StatusCompleted
				})).Return(scanBoolRow(true))
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "train-extras" && arr[1] == tracking.StatusCompleted
				})).Return(scanBoolRow(true))
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "train-win" && arr[1] == tracking.StatusCompleted
				})).Return(scanBoolRow(true))
			},
			stepID:  "auto_tune",
			wantOk:  true,
			wantMsg: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB := &mocks.MockDB{}
			if tc.setup != nil {
				tc.setup(mockDB)
			}
			ok, errMsg := CanRunPipelineStep(context.Background(), tc.stepID)
			require.Equal(t, tc.wantOk, ok)
			assert.Equal(t, tc.wantMsg, errMsg)
			mockDB.AssertExpectations(t)
		})
	}
}
