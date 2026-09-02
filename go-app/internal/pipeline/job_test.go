package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/mocks"
	steps "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
)

// row types for DB mock (implement db.Row)
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

type scanIntRow int

func (r scanIntRow) Scan(dest ...any) error {
	if len(dest) < 1 {
		return nil
	}
	if p, ok := dest[0].(*int); ok {
		*p = int(r)
		return nil
	}
	return nil
}

type scanErrRow struct{ err error }

func (r scanErrRow) Scan(_ ...any) error { return r.err }

func setupPipelineDB(t *testing.T, mockDB *mocks.MockDB) {
	t.Helper()
	db.SetDB(mockDB)
	t.Cleanup(func() { db.SetDB(nil) })
}

func TestRunJob(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	testCases := []struct {
		name    string
		setup   func(*mocks.MockDB)
		fn      JobFunc
		wantErr error
	}{
		{
			name: "pipeline_busy_returns_ErrPipelineBusy",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanBoolRow(true))
			},
			fn:      func(context.Context) (any, error) { return nil, nil },
			wantErr: ErrPipelineBusy,
		},
		{
			name: "start_fails_job_still_runs_returns_fn_error",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.MatchedBy(func(s string) bool {
					return strings.Contains(s, "SELECT EXISTS")
				}), mock.Anything).Return(scanBoolRow(false))
				m.On("QueryRow", mock.Anything, mock.MatchedBy(func(s string) bool {
					return strings.Contains(s, "INSERT INTO data_migrations")
				}), mock.Anything).Return(scanErrRow{err: errors.New("start failed")})
			},
			fn:      func(context.Context) (any, error) { return nil, errors.New("job error") },
			wantErr: errors.New("job error"),
		},
		{
			name: "job_fn_returns_error",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanBoolRow(false))
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanIntRow(1))
				m.On("Exec", mock.Anything, mock.Anything, mock.Anything).
					Return(nil)
			},
			fn:      func(context.Context) (any, error) { return nil, errors.New("job failed") },
			wantErr: errors.New("job failed"),
		},
		{
			name: "job_fn_success",
			setup: func(m *mocks.MockDB) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanBoolRow(false))
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
					Return(scanIntRow(1))
				m.On("Exec", mock.Anything, mock.Anything, mock.Anything).
					Return(nil)
			},
			fn:      func(context.Context) (any, error) { return "ok", nil },
			wantErr: nil,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			mockDB := &mocks.MockDB{}
			tc.setup(mockDB)
			ctx := context.Background()
			err := RunJob(ctx, "test-job", nil, 0, tc.fn)
			switch {
			case tc.wantErr == ErrPipelineBusy:
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrPipelineBusy))
			case tc.wantErr != nil:
				require.Error(t, err)
				assert.Equal(t, tc.wantErr.Error(), err.Error())
			default:
				require.NoError(t, err)
			}
			mockDB.AssertExpectations(t)
		})
	}
}

func TestLaneBusy(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	mockDB := &mocks.MockDB{}
	setupPipelineDB(t, mockDB)
	mockDB.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
		Return(scanBoolRow(true))
	busy, err := LaneBusy(context.Background(), "xi-retrain")
	require.NoError(t, err)
	assert.True(t, busy)

	mockDB2 := &mocks.MockDB{}
	setupPipelineDB(t, mockDB2)
	mockDB2.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
		Return(scanBoolRow(false))
	busy, err = LaneBusy(context.Background(), "xi-retrain")
	require.NoError(t, err)
	assert.False(t, busy)
}

// TestLaneBusyCoversEveryComputeStep is the regression guard for the drift that once let
// a training step run alongside another: the lock consulted a hand-maintained command list
// a step had never been added to. It asserts over the registry rather than a named step, so
// it keeps guarding as steps come and go.
func TestLaneBusyCoversEveryComputeStep(t *testing.T) {
	t.Parallel()

	registry := steps.Steps()
	compute := registry.CommandsInLane(steps.LaneCompute)
	for _, step := range registry.All() {
		if step.EffectiveLane() != steps.LaneCompute {
			continue
		}
		assert.Contains(t, compute, step.Command,
			"%s is a compute step but does not hold the compute lane", step.ID)
	}
	assert.Contains(t, compute, "xi-retrain")
	assert.NotContains(t, registry.CommandsInLane(steps.LaneData), "xi-retrain")
}

func TestRunJob_WithTimeout(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	mockDB := &mocks.MockDB{}
	setupPipelineDB(t, mockDB)
	mockDB.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
		Return(scanBoolRow(false))
	mockDB.On("QueryRow", mock.Anything, mock.Anything, mock.Anything).
		Return(scanIntRow(1))
	mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).
		Return(nil)
	ctx := context.Background()
	err := RunJob(ctx, "test-job", nil, 10*time.Second, func(_ context.Context) (any, error) { return nil, nil })
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
}
