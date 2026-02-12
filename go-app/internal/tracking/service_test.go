package tracking

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db/mocks"
)

func TestTracker_CaptureExit(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Arrange
		mockDB := &mocks.DBMock{}
		db.SetDB(mockDB)
		tracker := &Tracker{ID: 1}
		ctx := context.Background()
		var runErr error

		mockDB.On("Exec", mock.Anything, mock.Anything, 1, StatusCompleted, mock.Anything, (*string)(nil)).Return(nil)

		// Act
		tracker.CaptureExit(ctx, &runErr, nil)

		// Assert
		mockDB.AssertExpectations(t)
	})

	t.Run("failure", func(t *testing.T) {
		// Arrange
		mockDB := &mocks.DBMock{}
		db.SetDB(mockDB)
		tracker := &Tracker{ID: 1}
		ctx := context.Background()
		runErr := errors.New("boom")

		mockDB.On("Exec", mock.Anything, mock.Anything, 1, StatusFailed, mock.Anything, mock.MatchedBy(func(s *string) bool {
			return s != nil && *s == "boom"
		})).
			Return(nil)

		// Act
		tracker.CaptureExit(ctx, &runErr, nil)

		// Assert
		mockDB.AssertExpectations(t)
	})

	t.Run("cancelled by context", func(t *testing.T) {
		// Arrange
		mockDB := &mocks.DBMock{}
		db.SetDB(mockDB)
		tracker := &Tracker{ID: 1}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var runErr error

		mockDB.On("Exec", mock.Anything, mock.Anything, 1, StatusCancelled, mock.Anything, (*string)(nil)).Return(nil)

		// Act
		tracker.CaptureExit(ctx, &runErr, nil)

		// Assert
		mockDB.AssertExpectations(t)
	})

	t.Run("cancelled by error", func(t *testing.T) {
		// Arrange
		mockDB := &mocks.DBMock{}
		db.SetDB(mockDB)
		tracker := &Tracker{ID: 1}
		ctx := context.Background()
		runErr := context.Canceled

		mockDB.On("Exec", mock.Anything, mock.Anything, 1, StatusCancelled, mock.Anything, (*string)(nil)).Return(nil)

		// Act
		tracker.CaptureExit(ctx, &runErr, nil)

		// Assert
		mockDB.AssertExpectations(t)
	})
}
