package main

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
