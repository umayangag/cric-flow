package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/tracking"
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

func setupPipelineDB(t *testing.T, mockDB *mocks.DBMock) {
	t.Helper()
	db.SetDB(mockDB)
	t.Cleanup(func() { db.SetDB(nil) })
}

func TestCanRunPipelineStep(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	cases := []struct {
		name    string
		setup   func(*mocks.DBMock)
		stepID  string
		wantOk  bool
		wantMsg string
	}{
		{
			name:    "unknown_step_returns_false",
			setup:   func(*mocks.DBMock) { db.SetDB(nil) },
			stepID:  "unknown_step",
			wantOk:  false,
			wantMsg: "unknown step",
		},
		{
			name: "another_running_returns_false",
			setup: func(m *mocks.DBMock) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).
					Return(scanBoolRow(true))
			},
			stepID:  "import",
			wantOk:  false,
			wantMsg: "another pipeline step is already running",
		},
		{
			name: "import_no_previous_runnable",
			setup: func(m *mocks.DBMock) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).
					Return(scanBoolRow(false))
			},
			stepID:  "import",
			wantOk:  true,
			wantMsg: "",
		},
		{
			name: "precompute_prev_done_runnable",
			setup: func(m *mocks.DBMock) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).
					Return(scanBoolRow(false))
				m.On("QueryRow", mock.Anything, mock.Anything, "cricsheet-import", tracking.StatusCompleted).
					Return(scanBoolRow(true))
			},
			stepID:  "precompute",
			wantOk:  true,
			wantMsg: "",
		},
		{
			name: "precompute_prev_not_done_not_runnable",
			setup: func(m *mocks.DBMock) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).
					Return(scanBoolRow(false))
				m.On("QueryRow", mock.Anything, mock.Anything, "cricsheet-import", tracking.StatusCompleted).
					Return(scanBoolRow(false))
			},
			stepID:  "precompute",
			wantOk:  false,
			wantMsg: "complete the previous step (Import) first",
		},
		{
			name: "auto_tune_allowed_when_no_one_running",
			setup: func(m *mocks.DBMock) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).
					Return(scanBoolRow(false))
				m.On("QueryRow", mock.Anything, mock.Anything, "train-fielding", tracking.StatusCompleted).
					Return(scanBoolRow(true))
			},
			stepID:  "auto_tune",
			wantOk:  true,
			wantMsg: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB := &mocks.DBMock{}
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
