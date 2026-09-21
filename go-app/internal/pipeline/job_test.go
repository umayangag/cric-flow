package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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

// claimSetup arms the mock database for one lane claim: the advisory lock, the busy
// check, the insert, and the commit.
type claimSetup struct {
	laneBusy  bool
	beginErr  error
	insertErr error
}

func armClaim(t *testing.T, setup claimSetup) (*mocks.MockDB, *mocks.MockTx) {
	t.Helper()
	mockDB := &mocks.MockDB{}
	mockTx := &mocks.MockTx{}
	setupPipelineDB(t, mockDB)

	if setup.beginErr != nil {
		mockDB.On("Begin", mock.Anything).Return(nil, setup.beginErr)
		return mockDB, mockTx
	}
	mockDB.On("Begin", mock.Anything).Return(mockTx, nil)
	mockTx.On("Exec", mock.Anything, mock.MatchedBy(containing("pg_advisory_xact_lock")), mock.Anything).
		Return(nil)
	mockTx.On("QueryRow", mock.Anything, mock.MatchedBy(containing("SELECT EXISTS")), mock.Anything, mock.Anything).
		Return(scanBoolRow(setup.laneBusy))
	// Maybe: a claim that finds the lane held never reaches the insert, which is the
	// point of the claim. TestRunJob_ALaneHeldByAnotherRunIsRefusedWithoutASecondRow
	// asserts that absence directly.
	insert := mockTx.On("QueryRow", mock.Anything, mock.MatchedBy(containing("INSERT INTO data_migrations")),
		mock.Anything, mock.Anything, mock.Anything).Maybe()
	if setup.insertErr != nil {
		insert.Return(scanErrRow{err: setup.insertErr})
	} else {
		insert.Return(scanIntRow(1))
	}
	mockTx.On("Commit", mock.Anything).Return(nil).Maybe()
	mockTx.On("Rollback", mock.Anything).Return(pgx.ErrTxClosed)
	return mockDB, mockTx
}

func containing(fragment string) func(string) bool {
	return func(sql string) bool { return strings.Contains(sql, fragment) }
}

func TestRunJob(t *testing.T) {
	// Do not use t.Parallel(); tests use db.SetDB (global).

	testCases := []struct {
		name       string
		setup      claimSetup
		noDatabase bool
		fnErr      error
		wantErr    error
		wantRan    bool
	}{
		{
			name:    "a lane already held answers busy and the job does not run",
			setup:   claimSetup{laneBusy: true},
			wantErr: ErrPipelineBusy,
			wantRan: false,
		},
		{
			// Fail closed. This used to log a warning and run the job untracked, so a
			// database that could not answer "is this lane free?" let a second run start
			// beside the first with nothing in run history saying so (GO-06).
			name:    "a claim that cannot be made refuses the job",
			setup:   claimSetup{beginErr: errors.New("connection refused")},
			wantErr: errors.New("connection refused"),
			wantRan: false,
		},
		{
			name:    "an insert that fails refuses the job",
			setup:   claimSetup{insertErr: errors.New("insert failed")},
			wantErr: errors.New("insert failed"),
			wantRan: false,
		},
		{
			// No pool at all is a different case from a pool that will not answer: there
			// is no shared state to contend for, so the job runs untracked as before.
			name:       "no database runs the job untracked",
			noDatabase: true,
			wantRan:    true,
		},
		{
			name:    "a job that fails returns its own error",
			fnErr:   errors.New("job failed"),
			wantErr: errors.New("job failed"),
			wantRan: true,
		},
		{
			name:    "a job that succeeds returns no error",
			wantRan: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			var mockDB *mocks.MockDB
			var mockTx *mocks.MockTx
			if tc.noDatabase {
				db.SetDB(nil)
				t.Cleanup(func() { db.SetDB(nil) })
			} else {
				mockDB, mockTx = armClaim(t, tc.setup)
				mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
			}

			ran := false
			err := RunJob(context.Background(), "test-job", nil, 0, func(context.Context) (any, error) {
				ran = true
				return "ok", tc.fnErr
			})

			assertJobOutcome(t, err, tc.wantErr)
			assert.Equal(t, tc.wantRan, ran, "whether the job body ran")
			assertMocks(t, mockDB, mockTx)
		})
	}
}

func assertJobOutcome(t *testing.T, got, want error) {
	t.Helper()
	if want == nil {
		require.NoError(t, got)
		return
	}
	require.Error(t, got)
	if errors.Is(want, ErrPipelineBusy) {
		assert.ErrorIs(t, got, ErrPipelineBusy)
		return
	}
	assert.Contains(t, got.Error(), want.Error())
}

func assertMocks(t *testing.T, mockDB *mocks.MockDB, mockTx *mocks.MockTx) {
	t.Helper()
	if mockDB == nil {
		return
	}
	mockDB.AssertExpectations(t)
	mockTx.AssertExpectations(t)
}

// TestRunJob_ALaneHeldByAnotherRunIsRefusedWithoutASecondRow is GO-06's first half.
// The check and the insert were two statements with nothing between them, so two
// requests arriving inside one round trip both read "not busy" and both inserted. The
// claim is now one transaction under an advisory lock, and the second claimant sees the
// first's row — which is what this forces: the busy check answers true, and no INSERT
// may be issued.
func TestRunJob_ALaneHeldByAnotherRunIsRefusedWithoutASecondRow(t *testing.T) {
	// Do not use t.Parallel(); this test uses db.SetDB (global).

	mockDB := &mocks.MockDB{}
	mockTx := &mocks.MockTx{}
	setupPipelineDB(t, mockDB)
	mockDB.On("Begin", mock.Anything).Return(mockTx, nil)
	mockTx.On("Exec", mock.Anything, mock.MatchedBy(containing("pg_advisory_xact_lock")), mock.Anything).
		Return(nil)
	mockTx.On("QueryRow", mock.Anything, mock.MatchedBy(containing("SELECT EXISTS")), mock.Anything, mock.Anything).
		Return(scanBoolRow(true))
	mockTx.On("Rollback", mock.Anything).Return(pgx.ErrTxClosed)

	err := RunJob(context.Background(), "xi-retrain", nil, 0, func(context.Context) (any, error) {
		return nil, nil
	})

	require.ErrorIs(t, err, ErrPipelineBusy)
	mockTx.AssertNotCalled(t, "QueryRow", mock.Anything,
		mock.MatchedBy(containing("INSERT INTO data_migrations")), mock.Anything, mock.Anything, mock.Anything)
	mockTx.AssertNotCalled(t, "Commit", mock.Anything)
	mockDB.AssertExpectations(t)
	mockTx.AssertExpectations(t)
}

// TestLaneLockKey_IsStableAndPerLane guards the key the advisory lock is taken on: two
// lanes must not share one (they would serialise runs the lanes exist to overlap), and
// the key must not move between builds or processes, since the importer CLI takes the
// same lock from a different process.
func TestLaneLockKey_IsStableAndPerLane(t *testing.T) {
	t.Parallel()

	assert.Equal(t, LaneLockKey(steps.LaneCompute), LaneLockKey(steps.LaneCompute))
	assert.NotEqual(t, LaneLockKey(steps.LaneCompute), LaneLockKey(steps.LaneData))
	assert.Equal(t, uint32(0x811c9dc5), LaneLockKey(""), "FNV-1a offset basis; the hash must not be swapped")
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

	mockDB, mockTx := armClaim(t, claimSetup{})
	mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	err := RunJob(context.Background(), "test-job", nil, 10*time.Second,
		func(_ context.Context) (any, error) { return nil, nil })

	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockTx.AssertExpectations(t)
}
