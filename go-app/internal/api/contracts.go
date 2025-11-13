package api

import (
	"context"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/contracts"
)

// Client abstracts ML prediction client used by API handlers.
type Client interface {
	PredictBatting(ctx context.Context, feats []contracts.BattingFeatures) ([]contracts.BattingPrediction, error)
	PredictBowling(ctx context.Context, feats []contracts.BowlingFeatures) ([]contracts.BowlingPrediction, error)
}

type playerResponse struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"player_name"`
	IsWicketKeeper     int16    `json:"is_wicket_keeper"`
	IsRetired          int16    `json:"is_retired"`
	BattingConsistency *float32 `json:"batting_consistency,omitempty"`
	BowlingConsistency *float32 `json:"bowling_consistency,omitempty"`
}

type matchDetailsResponse struct {
	ID             int64   `json:"id"`
	MatchID        int64   `json:"match_id"`
	VenueID        *int64  `json:"venue_id"`
	OppositionID   *int64  `json:"opposition_id"`
	SeasonID       *int64  `json:"season_id"`
	Toss           *string `json:"toss"`
	BattingSession *string `json:"batting_session"`
	BowlingSession *string `json:"bowling_session"`
}

type precomputeRequest struct {
	Season  string   `json:"season"`
	Formats []string `json:"formats"`
}

type cricSheetRequest struct {
	Dir                  string `json:"dir"`
	PlaceholdersWeather  bool   `json:"placeholders_weather"`
	PlaceholdersFielding bool   `json:"placeholders_fielding"`
}
