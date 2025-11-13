package wx

import "context"

// Record represents a weather snapshot for a match and session.
// Keep it simple and portable for unit tests.
type Record struct {
	MatchID   int64
	Session   string // e.g., "batting" or "bowling"
	Temp      int
	Wind      int
	Rain      int
	Humidity  int
	Cloud     int
	Pressure  int
	Viscosity string // e.g., dry/humid/windy; service may normalize
}

// Provider fetches weather information for a given match from an external source.
//
//go:generate mockery --name Provider --output internal/mocks --case underscore
type Provider interface {
	Fetch(ctx context.Context, matchID int64) ([]Record, error)
}
