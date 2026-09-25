package db

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
	"github.com/umayangag/cric-flow/go-app/internal/venues"
)

// concurrentUpsertWorkers is how many goroutines are parked on the barrier below and then
// released together. It is deliberately smaller than the pool's MaxConns (10), because the
// blocker holds one connection and the barrier's own poll needs another.
const concurrentUpsertWorkers = 6

// TestVenueUniqueKeys_OnlyTheUpsertArbiter_Integration pins B-23's root cause, without
// depending on any interleaving.
//
// GetOrCreateVenue upserts `ON CONFLICT (normalized_name)`. ON CONFLICT arbitrates only
// the index it names: a conflict discovered while writing any *other* unique index on the
// same table is not absorbed, it is raised as SQLSTATE 23505 and the match file fails.
// `venue` carried `venue_venue_name_key UNIQUE (venue_name)` from the 0001 baseline, and
// IMPORT-08 (0021) moved the conflict target onto `normalized_name` without removing it --
// unlike 0004, which dropped `player_player_name_key` and `opposition_opposition_name_key`
// when it moved those two targets.
//
// So the invariant is: the only unique keys on `venue` are the primary key and the arbiter
// the upsert names. Any other one is a latent 23505 on the import path.
func TestVenueUniqueKeys_OnlyTheUpsertArbiter_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForVenues(t)

	rows, err := Pool.Query(ctx, `SELECT index_class.relname
		FROM pg_index
		JOIN pg_class AS index_class ON index_class.oid = pg_index.indexrelid
		JOIN pg_class AS table_class ON table_class.oid = pg_index.indrelid
		JOIN pg_namespace ON pg_namespace.oid = table_class.relnamespace
		WHERE pg_namespace.nspname = 'public'
		  AND table_class.relname = 'venue'
		  AND pg_index.indisunique
		ORDER BY index_class.relname`)
	require.NoError(t, err)
	defer rows.Close()

	uniqueKeys := []string{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		uniqueKeys = append(uniqueKeys, name)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []string{"ux_venue_normalized_name", "venue_pkey"}, uniqueKeys,
		"a unique key on venue that ON CONFLICT (normalized_name) does not arbitrate is "+
			"raised as 23505 on a concurrent import instead of being absorbed (B-23)")
}

// TestGetOrCreateVenue_ConcurrentFirstSightingsOfOneGround_Integration is B-23 itself: a
// full import from an empty database lost seven files and, with -fail-fast on by default,
// `make import` stopped at the first of them.
//
// The interleaving is forced, not waited for. An upsert cannot be paused between its
// arbiter pre-check and its index writes from outside the server, so the fixture parks
// every worker *inside* the pre-check and then releases them all in one wake-up:
//
//  1. A blocker transaction inserts the ground plainly and stays open. Its tuple is dirty,
//     so every worker's `ON CONFLICT (normalized_name)` pre-check finds it and waits on the
//     blocker's transaction id -- which pg_locks reports, so the barrier is observed rather
//     than slept on.
//  2. Once all six workers are confirmed waiting, the blocker rolls back. Every worker
//     wakes from the same lock release, re-runs the pre-check, finds nothing (the blocker's
//     tuple is aborted), and goes on to write its index entries.
//
// That is precisely the state a fresh import reaches when two files first name one ground
// at the same moment. With `venue_venue_name_key` still on the table the workers collide
// on it -- it sorts before the arbiter by oid, so it is written first -- and the losing
// worker gets `duplicate key value violates unique constraint "venue_venue_name_key"`.
// With the arbiter as the only unique key over the upserted columns, the collision is
// absorbed and every worker gets the same id.
//
// Four grounds are run because a worker that happens to finish before its neighbour
// re-checks is still absorbed; the defect needs two of the six to overlap, and four
// releases make that certain in practice, while the fixed code cannot fail on any
// interleaving at all.
func TestGetOrCreateVenue_ConcurrentFirstSightingsOfOneGround_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	ctx := connectAndMigrateForVenues(t)

	testCases := []struct {
		name  string
		venue string
		// cities are the cities the colliding match files place the ground in. 66 of the
		// archive's 892 grounds are spelled identically by files naming different cities,
		// and the cache keys on ground plus city, so those are the lookups that reach the
		// database more than once.
		cities []string
	}{
		{
			name:   "one ground, one city, six files at once",
			venue:  "Sharjah Cricket Stadium",
			cities: []string{"Sharjah"},
		},
		{
			name:   "one ground two files place in different cities",
			venue:  "National Stadium",
			cities: []string{"Karachi", "Bridgetown"},
		},
		{
			name:   "a city some files leave out",
			venue:  "M Chinnaswamy Stadium",
			cities: []string{"Bengaluru", "Bangalore", ""},
		},
		{
			name:   "a ground no file gives a city",
			venue:  "Gymkhana Club Ground",
			cities: []string{""},
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			ids, errs := upsertVenueFromEveryWorkerAtOnce(ctx, t, testCase.venue, testCase.cities)

			for worker := range errs {
				assert.NoErrorf(t, errs[worker],
					"worker %d lost the file: a concurrent first sighting of %q must be "+
						"absorbed by the upsert, not raised (B-23)", worker, testCase.venue)
			}
			assert.Len(t, distinctIDs(ids), 1,
				"every worker must come back with the one ground's one id")
			assert.Equal(t, 1, countVenueRowsForGround(ctx, t, testCase.venue),
				"one ground is one row however many files first name it at once")
		})
	}
}

// upsertVenueFromEveryWorkerAtOnce runs GetOrCreateVenue for one ground from
// concurrentUpsertWorkers goroutines that are all released from the same lock wake-up, and
// returns what each of them got. cities is cycled over the workers.
//
// It never fails the test on the workers' behalf: the caller asserts on the errors, which
// is the whole point of the fixture.
func upsertVenueFromEveryWorkerAtOnce(
	ctx context.Context,
	t *testing.T,
	venueName string,
	cities []string,
) (ids []int64, errs []error) {
	t.Helper()

	blocker := holdVenueRowUncommitted(ctx, t, venueName)

	ids = make([]int64, concurrentUpsertWorkers)
	errs = make([]error, concurrentUpsertWorkers)
	var workers sync.WaitGroup
	for worker := range concurrentUpsertWorkers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			ids[worker], errs[worker] = GetOrCreateVenue(ctx, venueName, cities[worker%len(cities)])
		}()
	}

	waitUntilWorkersAreParked(ctx, t, blocker.backendPID)
	blocker.release()
	workers.Wait()
	return ids, errs
}

// parkedBlocker is an open transaction holding an uncommitted `venue` row, plus the way to
// let go of it.
type parkedBlocker struct {
	backendPID int32
	release    func()
}

// holdVenueRowUncommitted inserts the ground on a dedicated connection inside a
// transaction deliberately left open, so every upsert of the same ground parks in its ON
// CONFLICT pre-check waiting on this transaction id.
//
// It writes the same identity key the workers will compute -- that is what makes their
// pre-check find it -- and it is rolled back, never committed, because the workers must
// wake to an empty table so that each of them takes the insert path rather than the DO
// UPDATE path. Cleanup releases it again in case a failed require skips the release below.
func holdVenueRowUncommitted(ctx context.Context, t *testing.T, venueName string) parkedBlocker {
	t.Helper()

	conn, err := Pool.Acquire(ctx)
	require.NoError(t, err)

	tx, err := conn.Begin(ctx)
	require.NoError(t, err)

	var backendPID int32
	require.NoError(t, tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&backendPID))
	_, err = tx.Exec(ctx,
		`INSERT INTO venue(venue_name, normalized_name) VALUES ($1, $2)`,
		venueName, venues.NormalizeName(venueName))
	require.NoError(t, err)

	var released sync.Once
	release := func() {
		released.Do(func() {
			_ = tx.Rollback(ctx)
			conn.Release()
		})
	}
	t.Cleanup(release)
	return parkedBlocker{backendPID: backendPID, release: release}
}

// waitUntilWorkersAreParked blocks until every worker waits on the blocker's transaction
// id. That is the barrier: it is read from pg_locks rather than assumed after a sleep, so
// the release that follows is what starts all six upserts, not the scheduler.
func waitUntilWorkersAreParked(ctx context.Context, t *testing.T, blockerPID int32) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	parked := 0
	for parked < concurrentUpsertWorkers {
		require.Truef(t, time.Now().Before(deadline),
			"only %d of %d workers parked on the blocker's transaction",
			parked, concurrentUpsertWorkers)
		time.Sleep(5 * time.Millisecond)
		require.NoError(t, Pool.QueryRow(ctx, `SELECT count(*) FROM pg_locks AS waiter
			WHERE waiter.locktype = 'transactionid'
			  AND NOT waiter.granted
			  AND waiter.transactionid IN (
			      SELECT held.transactionid FROM pg_locks AS held
			      WHERE held.pid = $1 AND held.locktype = 'transactionid' AND held.granted)`,
			blockerPID).Scan(&parked))
	}
}

// distinctIDs is the set of ids the workers came back with, so one assertion can say "they
// all agreed" without a per-worker comparison against a worker that may itself have failed
// and returned nothing.
func distinctIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	distinct := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, already := seen[id]; already {
			continue
		}
		seen[id] = struct{}{}
		distinct = append(distinct, id)
	}
	return distinct
}

// countVenueRowsForGround counts the rows this ground produced, keyed by its identity so a
// rival row created under a second spelling is counted too.
func countVenueRowsForGround(ctx context.Context, t *testing.T, venueName string) int {
	t.Helper()
	var rows int
	require.NoError(t, Pool.QueryRow(ctx,
		`SELECT count(*) FROM venue WHERE normalized_name = $1`,
		venues.NormalizeName(venueName)).Scan(&rows))
	return rows
}
