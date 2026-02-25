// Package server contains HTTP server app wiring and handlers for the API.
package server

import (
	"context"
	"sync"
)

// App holds long-lived application dependencies to be shared with handlers.
// Extend this struct as new dependencies are introduced.
type App struct {
	mlClient   Client
	dbProbe    DBProbe
	jobContext context.Context // cancelled on shutdown so pipeline jobs can exit gracefully

	// currentJobCancel is the cancel func for the running pipeline job (if any). Used by Stop pipeline.
	currentJobCancelMu sync.Mutex
	currentJobCancel   context.CancelFunc
}

// NewApp creates an App. jobCtx is cancelled when the process receives SIGTERM/SIGINT;
// pipeline jobs use it so they stop cleanly during shutdown. Pass nil in tests for context.Background() behavior.
func NewApp(jobCtx context.Context, client Client) *App {
	return &App{
		mlClient:   client,
		dbProbe:    newProductionDBProbe(),
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

// SetCurrentJobCancel stores the cancel func for the running pipeline job. Call when starting a job.
func (a *App) SetCurrentJobCancel(cancel context.CancelFunc) {
	if a == nil {
		return
	}
	a.currentJobCancelMu.Lock()
	defer a.currentJobCancelMu.Unlock()
	a.currentJobCancel = cancel
}

// ClearCurrentJobCancel clears the stored cancel func. Call in defer when the job goroutine exits.
func (a *App) ClearCurrentJobCancel() {
	if a == nil {
		return
	}
	a.currentJobCancelMu.Lock()
	defer a.currentJobCancelMu.Unlock()
	a.currentJobCancel = nil
}

// CancelCurrentJob cancels the current pipeline job context if one is running (e.g. user clicked Stop).
func (a *App) CancelCurrentJob() {
	if a == nil {
		return
	}
	a.currentJobCancelMu.Lock()
	fn := a.currentJobCancel
	a.currentJobCancel = nil
	a.currentJobCancelMu.Unlock()
	if fn != nil {
		fn()
	}
}
