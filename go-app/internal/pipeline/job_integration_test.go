package pipeline

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
	"github.com/umayangag/cric-flow/go-app/internal/tracking"
)

// The lane lock against a real Postgres (GO-06). The unit tests prove RunJob issues the
// advisory lock, the busy check and the insert as one transaction; only this proves the
// database actually serialises two claimants that arrive together — which is the whole
// claim, and the reason the lock is in the database rather than in this process.
//
//	make -C go-app test-db

const claimedCommand = "xi-retrain"

func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../migrations"))
}

func setupClaimDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Connect(ctx)
	require.NoError(t, err, "integration tests need POSTGRES_* pointing at a database")
	require.NoError(t, db.RunMigrations(ctx, migrationsDir()))
	require.NoError(t, db.Exec(ctx, `DELETE FROM data_migrations`))
}

func inProgressRows(t *testing.T, command string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(context.Background(), `
		SELECT COUNT(*)::int FROM data_migrations WHERE command = $1 AND status = $2
	`, command, tracking.StatusInProgress).Scan(&n))
	return n
}

// TestRunJob_ConcurrentClaimsStartExactlyOneRun_Integration is the race GO-06 names:
// "two POST /ops/pipeline/run/retrain within one round trip both start".
//
// Eight claimants are launched at once. The winner's job body blocks, so it is still
// holding the lane while the other seven claim — which is what makes this deterministic
// rather than a hope about scheduling: the losers cannot finish before the winner
// starts, so every one of them must meet an IN_PROGRESS row. Exactly one may enter the
// body; the other seven must be refused, and the table must hold one row.
func TestRunJob_ConcurrentClaimsStartExactlyOneRun_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	setupClaimDB(t)

	const claimants = 8
	release := make(chan struct{})
	entered := make(chan struct{}, claimants)
	results := make(chan error, claimants)

	for i := 0; i < claimants; i++ {
		go func() {
			results <- RunJob(context.Background(), claimedCommand, nil, 0,
				func(context.Context) (any, error) {
					entered <- struct{}{}
					<-release
					return nil, nil
				})
		}()
	}

	// The refused claimants return without waiting on anything. Reading exactly
	// claimants-1 of them before releasing the winner is what pins "only one got in":
	// had two been let through, only six would arrive and this would time out.
	for i := 0; i < claimants-1; i++ {
		select {
		case err := <-results:
			require.ErrorIs(t, err, ErrPipelineBusy,
				"a claimant that did not get the lane must be refused, not run")
		case <-time.After(30 * time.Second):
			require.Failf(t, "more than one claimant was let into the lane",
				"only %d of %d claimants were refused", i, claimants-1)
		}
	}
	assert.Len(t, entered, 1, "exactly one job body may run")
	assert.Equal(t, 1, inProgressRows(t, claimedCommand), "exactly one row may be IN_PROGRESS")

	close(release)
	require.NoError(t, <-results)
	assert.Zero(t, inProgressRows(t, claimedCommand), "the winner closes its own row out")
}

// TestRunJob_TheLanesStillOverlap_Integration is the guard on the lock's granularity:
// the whole point of the second lane is that a download and a training run overlap, so
// a lock that serialised everything would be a regression dressed as a fix.
func TestRunJob_TheLanesStillOverlap_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	setupClaimDB(t)

	computeHolds := make(chan struct{})
	releaseCompute := make(chan struct{})
	computeDone := make(chan error, 1)
	go func() {
		computeDone <- RunJob(context.Background(), claimedCommand, nil, 0,
			func(context.Context) (any, error) {
				close(computeHolds)
				<-releaseCompute
				return nil, nil
			})
	}()
	<-computeHolds

	dataRan := false
	err := RunJob(context.Background(), "dataset-fetch", nil, 0, func(context.Context) (any, error) {
		dataRan = true
		return nil, nil
	})

	close(releaseCompute)
	require.NoError(t, <-computeDone)
	require.NoError(t, err)
	assert.True(t, dataRan, "the data lane must not wait on the compute lane")
}
