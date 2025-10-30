package cricinfo

// MatchListItem represents a single innings row from the Cricinfo team results page.
type MatchListItem struct {
	Score       string
	Wickets     int
	Overs       string
	RPO         string
	Target      string
	Inning      string
	Result      string
	Opposition  string
	Ground      string
	DateText    string
	MatchID     string
	URLText     string
}
