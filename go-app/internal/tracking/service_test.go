package tracking

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/mocks"
)

func TestTracker_CaptureExit(t *testing.T) {
	t.Run("nil_receiver_no_op", func(_ *testing.T) {
		var tracker *Tracker
		tracker.CaptureExit(context.Background(), nil, nil)
	})

	t.Run("success", func(t *testing.T) {
		// Arrange
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		tracker := &Tracker{ID: 1}
		ctx := context.Background()
		var runErr error

		mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// Act
		tracker.CaptureExit(ctx, &runErr, nil)

		// Assert
		mockDB.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		// Arrange
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		tracker := &Tracker{ID: 1}
		ctx := context.Background()
		runErr := errors.New("boom")

		mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// Act
		tracker.CaptureExit(ctx, &runErr, nil)

		// Assert
		mockDB.AssertExpectations(t)
	})

	t.Run("cancelled by context", func(t *testing.T) {
		// Arrange
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		tracker := &Tracker{ID: 1}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var runErr error

		mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// Act
		tracker.CaptureExit(ctx, &runErr, nil)

		// Assert
		mockDB.AssertExpectations(t)
	})

	t.Run("cancelled by error", func(t *testing.T) {
		// Arrange
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		tracker := &Tracker{ID: 1}
		ctx := context.Background()
		runErr := context.Canceled

		mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// Act
		tracker.CaptureExit(ctx, &runErr, nil)

		// Assert
		mockDB.AssertExpectations(t)
	})
}

func TestTracker_Complete_Fail_Cancel(t *testing.T) {
	t.Run("complete_success", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })
		mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		tracker := &Tracker{ID: 1}
		err := tracker.Complete(context.Background(), map[string]string{"k": "v"})
		require.NoError(t, err)
		mockDB.AssertExpectations(t)
	})

	t.Run("fail_success", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })
		mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		tracker := &Tracker{ID: 1}
		err := tracker.Fail(context.Background(), "failed")
		require.NoError(t, err)
		mockDB.AssertExpectations(t)
	})

	t.Run("cancel_success", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })
		mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		tracker := &Tracker{ID: 1}
		err := tracker.Cancel(context.Background())
		require.NoError(t, err)
		mockDB.AssertExpectations(t)
	})
}

func TestTracker_TryComplete_TryFail_NilSafe(_ *testing.T) {
	var tracker *Tracker
	tracker.TryComplete(context.Background(), nil)
	tracker.TryFail(context.Background(), "ignored")
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

// TestStartExclusive pins the atomic claim that replaced the check-then-insert lane
// lock (GO-06): the busy check and the insert are one transaction, taken under an
// advisory lock, and a lane that is already held yields ErrRunConflict rather than a
// second row.
func TestStartExclusive(t *testing.T) {
	// Do not use t.Parallel(): these tests set the package-level db.

	testCases := []struct {
		name          string
		withDatabase  bool
		laneBusy      bool
		conflicting   []string
		wantErr       error
		wantID        int
		wantCommitted bool
	}{
		{
			name:          "a free lane is claimed and the run recorded",
			withDatabase:  true,
			conflicting:   []string{"xi-retrain", "cricsheet-import"},
			wantID:        42,
			wantCommitted: true,
		},
		{
			name:         "a held lane is refused",
			withDatabase: true,
			laneBusy:     true,
			conflicting:  []string{"xi-retrain", "cricsheet-import"},
			wantErr:      ErrRunConflict,
		},
		{
			name:          "a command in no lane is recorded without a busy check",
			withDatabase:  true,
			conflicting:   nil,
			wantID:        7,
			wantCommitted: true,
		},
		{
			name:         "no database means nothing to claim",
			withDatabase: false,
			conflicting:  []string{"xi-retrain"},
			wantErr:      ErrNoDatabase,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			mockTx := &mocks.MockTx{}
			mockDB := &mocks.MockDB{}
			if tc.withDatabase {
				mockDB.On("Begin", mock.Anything).Return(mockTx, nil)
				mockTx.On("Exec", mock.Anything, mock.MatchedBy(func(s string) bool {
					return strings.Contains(s, "pg_advisory_xact_lock")
				}), mock.Anything).Return(nil)
				mockTx.On("QueryRow", mock.Anything, mock.MatchedBy(func(s string) bool {
					return strings.Contains(s, "SELECT EXISTS")
				}), mock.Anything, mock.Anything).Return(scanBoolRow(tc.laneBusy))
				mockTx.On("QueryRow", mock.Anything, mock.MatchedBy(func(s string) bool {
					return strings.Contains(s, "INSERT INTO data_migrations")
				}), mock.Anything, mock.Anything, mock.Anything).Return(scanIntRow(tc.wantID))
				mockTx.On("Commit", mock.Anything).Return(nil)
				mockTx.On("Rollback", mock.Anything).Return(pgx.ErrTxClosed)
				db.SetDB(mockDB)
			} else {
				db.SetDB(nil)
			}
			t.Cleanup(func() { db.SetDB(nil) })

			tracker, err := StartExclusive(
				context.Background(), "xi-retrain", map[string]string{"format": "T20"},
				laneLockKeyForTest, tc.conflicting)

			assertClaim(t, tracker, err, tc.wantErr, tc.wantID)
			mockTx.AssertNumberOfCalls(t, "Commit", commitCount(tc.wantCommitted))
		})
	}
}

// laneLockKeyForTest stands in for pipeline.LaneLockKey, which tracking must not import.
const laneLockKeyForTest uint32 = 0x1234abcd

func commitCount(committed bool) int {
	if committed {
		return 1
	}
	return 0
}

func assertClaim(t *testing.T, tracker *Tracker, err, wantErr error, wantID int) {
	t.Helper()
	if wantErr != nil {
		require.ErrorIs(t, err, wantErr)
		require.Nil(t, tracker)
		return
	}
	require.NoError(t, err)
	require.NotNil(t, tracker)
	require.Equal(t, wantID, tracker.ID)
}

// TestStartExclusive_ClaimFailureIsReturned is the fail-closed half: a database that
// will not answer used to be logged and treated as "the lane is free".
func TestStartExclusive_ClaimFailureIsReturned(t *testing.T) {
	// Do not use t.Parallel(): this test sets the package-level db.

	mockDB := &mocks.MockDB{}
	mockDB.On("Begin", mock.Anything).Return(nil, errors.New("connection refused"))
	db.SetDB(mockDB)
	t.Cleanup(func() { db.SetDB(nil) })

	tracker, err := StartExclusive(context.Background(), "xi-retrain", nil, 1, []string{"xi-retrain"})

	require.Error(t, err)
	assert.Nil(t, tracker)
	assert.NotErrorIs(t, err, ErrRunConflict, "an unanswerable claim is not a free lane")
	assert.Contains(t, err.Error(), "connection refused")
}
