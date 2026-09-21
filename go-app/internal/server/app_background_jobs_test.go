package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWaitForBackgroundJobs_WaitsForARunToRecordItsOutcome is the shutdown half of
// GO-05.
//
// `cmd/api` cancels the job context and then closes the pool. srv.Shutdown waits for
// in-flight HTTP requests, and a pipeline run is not one — it is a goroutine behind a
// 202 that still owns an IN_PROGRESS row — so the pool used to close while the run was
// still unwinding and the write that would have recorded the cancellation met a closed
// pool.
//
// The ordering is forced, not raced for: the job does not finish until this test says
// so, and the wait must still be blocked at that point.
func TestWaitForBackgroundJobs_WaitsForARunToRecordItsOutcome(t *testing.T) {
	t.Parallel()
	app := &App{}

	recorded := make(chan struct{})
	jobStarted := make(chan struct{})
	app.RunBackgroundJob(func() {
		close(jobStarted)
		<-recorded
	})
	<-jobStarted

	impatient, cancelImpatient := context.WithCancel(context.Background())
	cancelImpatient()
	assert.False(t, app.WaitForBackgroundJobs(impatient),
		"a job that has not recorded its outcome is not finished")

	close(recorded)
	require.True(t, app.WaitForBackgroundJobs(context.Background()),
		"once the run has recorded its outcome the pool may close")
}

// TestWaitForBackgroundJobs_WithNothingRunningReturnsImmediately keeps a shutdown with
// no pipeline work from paying the drain timeout.
func TestWaitForBackgroundJobs_WithNothingRunningReturnsImmediately(t *testing.T) {
	t.Parallel()

	assert.True(t, (&App{}).WaitForBackgroundJobs(context.Background()))
	assert.True(t, (*App)(nil).WaitForBackgroundJobs(context.Background()),
		"a nil App is the zero value handlers may hold in tests")
}
