// Package server contains HTTP server app wiring and handlers for the API.
package server

// App holds long-lived application dependencies to be shared with handlers.
// Extend this struct as new dependencies are introduced.
type App struct {
    mlClient Client
    dbProbe DBProbe
}

func NewApp(client Client) *App {
    return &App{
        mlClient: client,
        dbProbe:  newProductionDBProbe(),
    }
}
