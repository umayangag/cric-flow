package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStopRun_CallsCancelJobAndCancelMigration(t *testing.T) {
	var cancelJobCalled, cancelMigrationCalled bool
	cancelJob := func() { cancelJobCalled = true }
	cancelMigration := func(_ context.Context, _ string) (bool, error) {
		cancelMigrationCalled = true
		return true, nil
	}
	cancelled, err := StopRun(context.Background(), "test", cancelJob, cancelMigration)
	require.NoError(t, err)
	require.True(t, cancelled)
	require.True(t, cancelJobCalled)
	require.True(t, cancelMigrationCalled)
}

func TestStopRun_ReturnsCancelMigrationResult(t *testing.T) {
	cancelJob := func() {}
	cancelMigration := func(_ context.Context, _ string) (bool, error) {
		return false, errors.New("no run to cancel")
	}
	cancelled, err := StopRun(context.Background(), "user", cancelJob, cancelMigration)
	require.Error(t, err)
	require.False(t, cancelled)
}
