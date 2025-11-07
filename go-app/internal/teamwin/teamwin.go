// Package teamwin defines team-level features and a client interface to query
// a win-probability model served by the ML service. This is scaffolding to
// align the codebase with the HLD; real feature engineering and HTTP client
// logic will be added subsequently.
package teamwin

import "context"

// Features is a minimal placeholder for team-level features. In the follow-up
// implementation, this will be derived from aggregated player predictions,
// venue/opposition context, and weather.
type Features struct {
	MatchID int64
	Format  string // e.g., T20, ODI
	// Example placeholders
	TeamBattingStrength float64
	TeamBowlingStrength float64
}

// Response models the ML service output for win probability.
type Response struct {
	WinProbability float64
}

// Client abstracts the ML service endpoint for match win prediction.
type Client interface {
	Predict(ctx context.Context, f Features) (Response, error)
}

// NoopClient is a development stub that returns a neutral probability.
type NoopClient struct{}

func (NoopClient) Predict(ctx context.Context, f Features) (Response, error) {
	return Response{WinProbability: 0.5}, nil
}
