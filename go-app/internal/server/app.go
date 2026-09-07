// Package server contains HTTP server app wiring and handlers for the API.
package server

import (
	"context"
	"sync"

	"github.com/umayangag/cric-flow/go-app/internal/predictions"
	"github.com/umayangag/cric-flow/go-app/internal/services/opsstatus"
	pipelinesvc "github.com/umayangag/cric-flow/go-app/internal/services/pipeline"
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
	jobContext              context.Context // cancelled on shutdown so pipeline jobs can exit gracefully

	// jobCancels holds one cancel func per lane, for the job running in that lane.
	//
	// This used to be a single slot, which was correct only while one global lock
	// meant one job. Acquisition has its own lane (ops plan F-2) precisely so a
	// download and a training run can overlap — and the moment they do, a single
	// slot means starting the download silently makes the training run
	// uncancellable, and Stop cancels whichever job wrote the slot last. Keying by
	// lane makes the structure say what the lanes already promised.
	jobCancelsMu sync.Mutex
	jobCancels   map[pipelinesvc.Lane]context.CancelFunc

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

// SetJobCancel stores the cancel func for the job starting in a lane. Call when
// starting a job; the lane comes from the step's registry entry.
func (a *App) SetJobCancel(lane pipelinesvc.Lane, cancel context.CancelFunc) {
	if a == nil {
		return
	}
	a.jobCancelsMu.Lock()
	defer a.jobCancelsMu.Unlock()
	if a.jobCancels == nil {
		a.jobCancels = map[pipelinesvc.Lane]context.CancelFunc{}
	}
	a.jobCancels[lane] = cancel
}

// ClearJobCancel forgets the lane's cancel func. Call in defer when the job goroutine exits.
func (a *App) ClearJobCancel(lane pipelinesvc.Lane) {
	if a == nil {
		return
	}
	a.jobCancelsMu.Lock()
	defer a.jobCancelsMu.Unlock()
	delete(a.jobCancels, lane)
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
		if fn, ok := a.jobCancels[lane]; ok {
			fns = append(fns, fn)
			delete(a.jobCancels, lane)
		}
	}
	a.jobCancelsMu.Unlock()

	for _, fn := range fns {
		fn()
	}
	return len(fns)
}
