package main

// app holds long-lived application dependencies to be shared with handlers.
// Extend this struct as new dependencies are introduced.
type app struct {
	mlClient Client
}

func newApp(client Client) *app {
	return &app{
		mlClient: client,
	}
}
