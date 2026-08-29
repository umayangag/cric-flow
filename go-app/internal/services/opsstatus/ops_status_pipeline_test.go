package opsstatus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// latestRunRows implements db.Rows for the "most recent run of this command" lookup
// that decides whether a prerequisite step is complete. It yields one row, or none
// when status is empty, which is a command that has never run.
type latestRunRows struct {
	command  string
	status   tracking.MigrationStatus
	consumed bool
}

func (r *latestRunRows) Next() bool {
	if r.status == "" || r.consumed {
		return false
	}
	r.consumed = true
	return true
}

func (r *latestRunRows) Scan(dest ...any) error {
	if p, ok := dest[1].(*string); ok {
		*p = r.command
	}
	if p, ok := dest[3].(*time.Time); ok {
		*p = time.Now().UTC()
	}
	if p, ok := dest[5].(*tracking.MigrationStatus); ok {
		*p = r.status
	}
	return nil
}

func (r *latestRunRows) Close() {}

func (r *latestRunRows) Err() error { return nil }

// prerequisiteRun mocks the latest-run lookup for one command.
func prerequisiteRun(m *mocks.MockDB, command string, status tracking.MigrationStatus) {
	m.On("Query", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
		arr, ok := a.([]any)
		return ok && len(arr) >= 1 && arr[0] == command
	})).Return(&latestRunRows{command: command, status: status}, nil)
}

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

	testCases := []struct {
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
				prerequisiteRun(m, "cricsheet-import", tracking.StatusCompleted)
			},
			stepID:  "precompute",
			wantOk:  true,
			wantMsg: "",
		},
		{
			name: "precompute_prev_cancelled_not_runnable",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "precompute-features" && arr[1] == tracking.StatusInProgress
				})).Return(scanBoolRow(false))
				prerequisiteRun(m, "cricsheet-import", tracking.StatusCancelled)
			},
			stepID:  "precompute",
			wantOk:  false,
			wantMsg: "complete the previous step (Import) first",
		},
		{
			// The generalisation of the cancelled-precompute bug: it is the latest run
			// that decides, so a step whose newest run failed blocks what comes after
			// it however many times it succeeded before.
			name: "precompute_prev_failed_not_runnable",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "precompute-features" && arr[1] == tracking.StatusInProgress
				})).Return(scanBoolRow(false))
				prerequisiteRun(m, "cricsheet-import", tracking.StatusFailed)
			},
			stepID:  "precompute",
			wantOk:  false,
			wantMsg: "complete the previous step (Import) first",
		},
		{
			name: "precompute_prev_never_run_not_runnable",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "precompute-features" && arr[1] == tracking.StatusInProgress
				})).Return(scanBoolRow(false))
				prerequisiteRun(m, "cricsheet-import", "")
			},
			stepID:  "precompute",
			wantOk:  false,
			wantMsg: "complete the previous step (Import) first",
		},
		{
			// Auto-tune needs the export and nothing after it: it reads the same
			// inputs the train steps read, so requiring them would force the one
			// order the pipeline exists to avoid — train on stale params, then
			// search for better ones.
			name: "auto_tune_allowed_after_export_without_any_training",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "ml-auto-tune" && arr[1] == tracking.StatusInProgress
				})).Return(scanBoolRow(false))
				prerequisiteRun(m, "export-dataset", tracking.StatusCompleted)
			},
			stepID:  "auto_tune",
			wantOk:  true,
			wantMsg: "",
		},
		{
			name: "auto_tune_without_export_not_runnable",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.MatchedBy(func(a any) bool {
					arr, ok := a.([]any)
					return ok && len(arr) >= 2 && arr[0] == "ml-auto-tune" && arr[1] == tracking.StatusInProgress
				})).Return(scanBoolRow(false))
				prerequisiteRun(m, "export-dataset", "")
			},
			stepID:  "auto_tune",
			wantOk:  false,
			wantMsg: "complete the previous step (Export) first",
		},
	}

	for i := range testCases {
		tc := testCases[i]
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
