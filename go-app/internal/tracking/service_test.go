package tracking

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
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
		if err != nil {
			t.Fatal(err)
		}
		mockDB.AssertExpectations(t)
	})

	t.Run("fail_success", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })
		mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		tracker := &Tracker{ID: 1}
		err := tracker.Fail(context.Background(), "failed")
		if err != nil {
			t.Fatal(err)
		}
		mockDB.AssertExpectations(t)
	})

	t.Run("cancel_success", func(t *testing.T) {
		mockDB := &mocks.MockDB{}
		db.SetDB(mockDB)
		t.Cleanup(func() { db.SetDB(nil) })
		mockDB.On("Exec", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		tracker := &Tracker{ID: 1}
		err := tracker.Cancel(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		mockDB.AssertExpectations(t)
	})
}

func TestTracker_TryComplete_TryFail_NilSafe(_ *testing.T) {
	var tracker *Tracker
	tracker.TryComplete(context.Background(), nil)
	tracker.TryFail(context.Background(), "ignored")
}
