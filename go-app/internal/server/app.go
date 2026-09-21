// Package server contains HTTP server app wiring and handlers for the API.
package server

import (
	"context"
	"log/slog"
	"sync"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
	"github.com/umayangag/cric-flow/go-app/internal/predictions"
	"github.com/umayangag/cric-flow/go-app/internal/services/opsstatus"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
	"github.com/umayangag/cric-flow/go-app/internal/trackrecord"
)

// App holds long-lived application dependencies to be shared with handlers.
// Extend this struct as new dependencies are introduced.
type App struct {
	// mlClient is the one ml-service client: after P-5 every prediction the API serves is
	// an XI-layer call, so there is no second client and no second contract.
	mlClient *MLClient
	dbProbe  opsstatus.DBProbe
	// The prediction record (P2-3), as two dependencies rather than one store: the
	// prediction path may file an answer and may not read the record back, which is the
	// smaller of the two capabilities and the only one it needs. Nil is the process
	// default — the database-backed record — so a handler built without either still
	// works; tests set the field they are exercising to a mock.
	predictionRecorderStore predictions.Recorder
	predictionReaderStore   predictions.Reader
	// matchLookupStore is the track record's view of the match tables (P2-4), on the
	// same terms: nil is the database, tests set a fake.
	matchLookupStore trackrecord.MatchLookup
	// auctionStore is the auction record (P3-1), on the same terms again. One store and
	// not two halves: unlike a prediction, an auction is edited all the way through — the
	// operator lists, sells, and undoes a mistyped sale — so there is no read-only
	// capability to hand out separately.
	auctionStore auction.Store
	// auctionLookups is what the projection reads from the database beside the record
	// (P3-2): the grounds' names, and a side's last recorded eleven. Nil is the process
	// default; tests set a stub so the handlers can be exercised with no database.
	auctionLookups auctionLookups
	jobContext     context.Context // cancelled on shutdown so pipeline jobs can exit gracefully

	// jobCancels holds one registration per lane, for the job running in that lane.
	//
	// This used to be a single slot, which was correct only while one global lock
	// meant one job. Acquisition has its own lane (ops plan F-2) precisely so a
	// download and a training run can overlap — and the moment they do, a single
	// slot means starting the download silently makes the training run
	// uncancellable, and Stop cancels whichever job wrote the slot last. Keying by
	// lane makes the structure say what the lanes already promised.
	jobCancelsMu sync.Mutex
	jobCancels   map[pipelinesvc.Lane]*laneJob

	// backgroundJobs counts the goroutines started behind a 202 that still have run
	// history to write. Shutdown waits on it before the pool closes (GO-05).
	backgroundJobs sync.WaitGroup

	// planCancel stops a run plan as a whole, which is a different thing from
	// stopping the step it is currently on (ops plan R-1).
	planCancel planCancel
}

// NewApp creates an App. jobCtx is cancelled when the process receives SIGTERM/SIGINT;
// pipeline jobs use it so they stop cleanly during shutdown. Pass nil for jobCtx in tests
// for context.Background() behaviour, and nil for client to use the configured ml-service.
func NewApp(jobCtx context.Context, client *MLClient) *App {
	if client == nil {
		client = NewMLClient()
	}
	return &App{
		mlClient:   client,
		dbProbe:    opsstatus.NewProductionDBProbe(),
		jobContext: jobCtx,
	}
}

// JobContext returns the context to use for background pipeline jobs. Cancelled on shutdown.
// If not set, callers should use context.Background() (see RunJobWithAppContext).
func (a *App) JobContext() context.Context {
	if a != nil && a.jobContext != nil {
		return a.jobContext
	}
	return context.Background()
}

// laneJob is one lane's registered job. The cancel func lives behind a pointer so a
// registration has an identity: function values are not comparable in Go, so there is
// no other way for a job to ask "is the thing stored here still mine?".
type laneJob struct{ cancel context.CancelFunc }

// SetJobCancel registers the cancel func for the job starting in a lane and returns
// the release func that deregisters it. Call release in a defer when the job goroutine
// exits; the lane comes from the step's registry entry.
//
// Release clears this registration and no other. It used to be an unconditional
// `delete(a.jobCancels, lane)` paired with an unconditional `a.jobCancels[lane] =
// cancel`, so two jobs racing into the same lane traded places: the second overwrote
// the first's cancel func on the way in and deleted the survivor's on the way out, and
// Stop then cancelled neither (GO-06).
//
// A lane that already holds a live registration keeps it. The lane belongs to whoever
// claimed it, the second job is refused by pipeline.RunJob, and its release is a no-op
// so every caller can defer it unconditionally.
func (a *App) SetJobCancel(lane pipelinesvc.Lane, cancel context.CancelFunc) func() {
	if a == nil {
		return func() {}
	}
	a.jobCancelsMu.Lock()
	defer a.jobCancelsMu.Unlock()
	if a.jobCancels == nil {
		a.jobCancels = map[pipelinesvc.Lane]*laneJob{}
	}
	if held := a.jobCancels[lane]; held != nil {
		slog.Warn("pipeline: lane already has a cancellable job; keeping the one that claimed it",
			slog.String("lane", string(lane)))
		return func() {}
	}
	registration := &laneJob{cancel: cancel}
	a.jobCancels[lane] = registration
	return func() { a.releaseJobCancel(lane, registration) }
}

// releaseJobCancel forgets the lane's registration, but only if it is still the one
// the caller made.
func (a *App) releaseJobCancel(lane pipelinesvc.Lane, registration *laneJob) {
	a.jobCancelsMu.Lock()
	defer a.jobCancelsMu.Unlock()
	if a.jobCancels[lane] == registration {
		delete(a.jobCancels, lane)
	}
}

// RunBackgroundJob starts fn in a goroutine that shutdown will wait for.
//
// Every one of these is the tail of a request that has already answered 202, and each
// one owns a data_migrations row it has to close out. Shutdown cancelled them and then
// closed the pool without waiting, so the write that would have recorded the
// cancellation met a closed pool and the row stayed IN_PROGRESS (GO-05).
func (a *App) RunBackgroundJob(fn func()) {
	if a == nil {
		go fn()
		return
	}
	a.backgroundJobs.Add(1)
	go func() {
		defer a.backgroundJobs.Done()
		fn()
	}()
}

// WaitForBackgroundJobs blocks until every background job has returned, or until ctx
// is done. It reports whether they all finished.
//
// Called during shutdown between cancelling the job context and closing the pool, so a
// cancelled run records its outcome while there is still a database to record it in.
// If the wait times out the pool closes anyway — a shutdown that hangs on a job that
// will not stop is worse than a row the startup sweep will tidy.
func (a *App) WaitForBackgroundJobs(ctx context.Context) bool {
	if a == nil {
		return true
	}
	done := make(chan struct{})
	go func() {
		a.backgroundJobs.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// CancelJobsInLanes cancels the running job in each named lane and reports how many
// it cancelled. Passing no lanes cancels every lane, which is what an unqualified
// Stop means.
func (a *App) CancelJobsInLanes(lanes ...pipelinesvc.Lane) int {
	if a == nil {
		return 0
	}
	a.jobCancelsMu.Lock()
	if len(lanes) == 0 {
		lanes = lanes[:0]
		for lane := range a.jobCancels {
			lanes = append(lanes, lane)
		}
	}
	fns := make([]context.CancelFunc, 0, len(lanes))
	for _, lane := range lanes {
		if job := a.jobCancels[lane]; job != nil {
			fns = append(fns, job.cancel)
			delete(a.jobCancels, lane)
		}
	}
	a.jobCancelsMu.Unlock()

	for _, fn := range fns {
		fn()
	}
	return len(fns)
}
