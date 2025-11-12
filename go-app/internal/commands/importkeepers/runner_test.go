package importkeepers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	mocks "github.com/umayangag/cric-info-scrapers/go-app/internal/commands/importkeepers/mocks"
)

func TestPreview_Happy(t *testing.T) {
	ctx := context.Background()
	m := mocks.NewMockKeeperRepository(t)

	targets := map[string]int{
		"a": 1,
		"b": 0,
	}
	// Expect counts for each target
	m.EXPECT().CountPlayersByLowerName(mock.Anything, "a").Return(int64(2), nil)
	m.EXPECT().CountPlayersByLowerName(mock.Anything, "b").Return(int64(1), nil)

	r := NewRunner(m)
	err := r.Preview(ctx, targets, true)
	assert.NoError(t, err)
}

func TestPreview_ErrorOnCount(t *testing.T) {
	ctx := context.Background()
	m := mocks.NewMockKeeperRepository(t)

	targets := map[string]int{
		"john": 1,
	}
	expectedErr := errors.New("db down")
	m.EXPECT().CountPlayersByLowerName(mock.Anything, "john").Return(int64(0), expectedErr)

	r := NewRunner(m)
	err := r.Preview(ctx, targets, false)
	assert.ErrorIs(t, err, expectedErr)
}

func TestApply_Happy_NoZeroOthers(t *testing.T) {
	ctx := context.Background()
	m := mocks.NewMockKeeperRepository(t)

	targets := map[string]int{
		"sam": 1,
		"max": 0,
	}
	m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 1, "sam").Return(int64(1), nil)
	m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 0, "max").Return(int64(1), nil)
	// No expectation for ZeroKeepersExcept; if called unexpectedly, mock will fail AssertExpectations

	r := NewRunner(m)
	err := r.Apply(ctx, targets, false)
	assert.NoError(t, err)
}

func TestApply_Happy_ZeroOthers(t *testing.T) {
	ctx := context.Background()
	m := mocks.NewMockKeeperRepository(t)

	targets := map[string]int{
		"x": 1,
		"y": 1,
		"z": 0,
	}
	m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 1, "x").Return(int64(1), nil)
	m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 1, "y").Return(int64(1), nil)
	m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 0, "z").Return(int64(1), nil)
	m.EXPECT().ZeroKeepersExcept(mock.Anything, mock.MatchedBy(func(names []string) bool {
		// order is non-deterministic due to map iteration; check set equality
		set := map[string]struct{}{}
		for _, n := range names {
			set[n] = struct{}{}
		}
		_, hasX := set["x"]
		_, hasY := set["y"]
		_, hasZ := set["z"]
		return hasX && hasY && hasZ && len(names) == 3
	})).Return(int64(10), nil)

	r := NewRunner(m)
	err := r.Apply(ctx, targets, true)
	assert.NoError(t, err)
}

func TestApply_ErrorOnUpdate(t *testing.T) {
	ctx := context.Background()
	m := mocks.NewMockKeeperRepository(t)

	targets := map[string]int{
		"alice": 1,
		"bob":   0,
	}
	expectedErr := errors.New("update failed")
	// Simulate first update fails on whichever key is iterated first.
	m.
		EXPECT().
		SetIsWicketKeeperByLowerName(
			mock.Anything,                 // ctx
			mock.AnythingOfType("int"),    // value
			mock.AnythingOfType("string"), // name
		).
		Return(int64(0), expectedErr)

	r := NewRunner(m)
	err := r.Apply(ctx, targets, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update keeper for")
}

func TestApply_ErrorOnZeroOthers(t *testing.T) {
	ctx := context.Background()
	m := mocks.NewMockKeeperRepository(t)

	targets := map[string]int{
		"a": 1,
		"b": 0,
	}
	m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 1, "a").Return(int64(1), nil)
	m.EXPECT().SetIsWicketKeeperByLowerName(mock.Anything, 0, "b").Return(int64(1), nil)
	expectedErr := errors.New("zero failed")
	m.EXPECT().ZeroKeepersExcept(mock.Anything, mock.Anything).Return(int64(0), expectedErr)

	r := NewRunner(m)
	err := r.Apply(ctx, targets, true)
	assert.ErrorIs(t, err, expectedErr)
}
