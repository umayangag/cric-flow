package tracking

import (
	"context"
	"errors"
	"testing"

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

func TestStart(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })

		mockDB.On("QueryRow", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(scanIntRow(42))

		tr, err := Start(context.Background(), "xi-retrain", map[string]string{"format": "T20"})
		require.NoError(t, err)
		require.NotNil(t, tr)
		require.Equal(t, 42, tr.ID)
	})

	t.Run("db_unavailable", func(t *testing.T) {
		db.SetDB(nil)
		t.Cleanup(func() { db.SetDB(nil) })

		_, err := Start(context.Background(), "xi-reload", nil)
		require.Error(t, err)
	})
}
