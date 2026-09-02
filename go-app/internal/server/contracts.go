package server

type playerResponse struct {
	ID             int64  `json:"id"`
	Name           string `json:"player_name"`
	IsWicketKeeper int16  `json:"is_wicket_keeper"`
	IsRetired      int16  `json:"is_retired"`
}

type matchDetailsResponse struct {
	ID             int64   `json:"id"`
	MatchID        int64   `json:"match_id"`
	VenueID        *int64  `json:"venue_id"`
	OppositionID   *int64  `json:"opposition_id"`
	SeasonID       *int64  `json:"season_id"`
	Toss           *string `json:"toss"`
	BattingSession *string `json:"batting_session,omitempty"`
	BowlingSession *string `json:"bowling_session,omitempty"`
}

type cricSheetRequest struct {
	Dir                  string `json:"dir"`
	PlaceholdersFielding bool   `json:"placeholders_fielding"`
	// Refresh downloads the configured archive again even when the dataset directory
	// already holds it. Cricsheet republishes under the same URL, so "the data has
	// changed" is a thing an operator knows and the server cannot infer.
	Refresh bool `json:"refresh"`
}
