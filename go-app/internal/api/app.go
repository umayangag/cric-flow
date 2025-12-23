// Package api contains HTTP server app wiring and handlers for the API.
// revive:disable:var-naming — package name "api" is intentional and conventional here.
package api

// App holds long-lived application dependencies to be shared with handlers.
// Extend this struct as new dependencies are introduced.
type App struct {
	mlClient Client
}

func NewApp(client Client) *App {
	return &App{
		mlClient: client,
	}
}
