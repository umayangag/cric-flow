package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/tracking"
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

func setupPipelineDB(t *testing.T, mockDB *mocks.DBMock) {
	t.Helper()
	db.SetDB(mockDB)
	t.Cleanup(func() { db.SetDB(nil) })
}

func TestRunJob(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	cases := []struct {
		name    string
		setup   func(*mocks.DBMock)
		fn      JobFunc
		wantErr error
	}{
		{
			name: "pipeline_busy_returns_ErrPipelineBusy",
			setup: func(m *mocks.DBMock) {
				setupPipelineDB(t, m)
				// HasInProgressForAnyCommand: QueryRow(ctx, sql, status, commands) -> 4 args
				m.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).Return(scanBoolRow(true))
			},
			fn:      func(context.Context) (any, error) { return nil, nil },
			wantErr: ErrPipelineBusy,
		},
		{
			name: "start_fails_job_still_runs_returns_fn_error",
			setup: func(m *mocks.DBMock) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).Return(scanBoolRow(false))
				// CreateMigration: QueryRow(ctx, sql, command, args, status) -> 5 args
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(scanErrRow{err: errors.New("start failed")})
			},
			fn:      func(context.Context) (any, error) { return nil, errors.New("job error") },
			wantErr: errors.New("job error"),
		},
		{
			name: "job_fn_returns_error",
			setup: func(m *mocks.DBMock) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).Return(scanBoolRow(false))
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(scanIntRow(1))
				m.On("Exec", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
			fn:      func(context.Context) (any, error) { return nil, errors.New("job failed") },
			wantErr: errors.New("job failed"),
		},
		{
			name: "job_fn_success",
			setup: func(m *mocks.DBMock) {
				setupPipelineDB(t, m)
				m.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).Return(scanBoolRow(false))
				m.On("QueryRow", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(scanIntRow(1))
				m.On("Exec", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
			fn:      func(context.Context) (any, error) { return "ok", nil },
			wantErr: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB := &mocks.DBMock{}
			tc.setup(mockDB)
			ctx := context.Background()
			err := RunJob(ctx, "test-job", nil, 0, tc.fn)
			if tc.wantErr == ErrPipelineBusy {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrPipelineBusy))
			} else if tc.wantErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.wantErr.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}
			mockDB.AssertExpectations(t)
		})
	}
}

func TestHasPipelineBusy(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	mockDB := &mocks.DBMock{}
	setupPipelineDB(t, mockDB)
	mockDB.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).Return(scanBoolRow(true))
	busy, err := HasPipelineBusy(context.Background())
	require.NoError(t, err)
	assert.True(t, busy)

	mockDB2 := &mocks.DBMock{}
	setupPipelineDB(t, mockDB2)
	mockDB2.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).Return(scanBoolRow(false))
	busy, err = HasPipelineBusy(context.Background())
	require.NoError(t, err)
	assert.False(t, busy)
}

func TestRunJob_WithTimeout(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	mockDB := &mocks.DBMock{}
	setupPipelineDB(t, mockDB)
	mockDB.On("QueryRow", mock.Anything, mock.Anything, tracking.StatusInProgress, mock.Anything).Return(scanBoolRow(false))
	mockDB.On("QueryRow", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(scanIntRow(1))
	mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	ctx := context.Background()
	err := RunJob(ctx, "test-job", nil, 10*time.Second, func(ctx context.Context) (any, error) { return nil, nil })
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
}
