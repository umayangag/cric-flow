// Package server contains HTTP server app wiring and handlers for the API.
package server

import "context"

// App holds long-lived application dependencies to be shared with handlers.
// Extend this struct as new dependencies are introduced.
type App struct {
	mlClient   Client
	dbProbe    DBProbe
	jobContext context.Context // cancelled on shutdown so pipeline jobs can exit gracefully
}

// NewApp creates an App. jobCtx is cancelled when the process receives SIGTERM/SIGINT;
// pipeline jobs use it so they stop cleanly during shutdown. Pass nil in tests for context.Background() behavior.
func NewApp(client Client, jobCtx context.Context) *App {
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
