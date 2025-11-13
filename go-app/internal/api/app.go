// Package api contains HTTP server app wiring and handlers for the API.
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
