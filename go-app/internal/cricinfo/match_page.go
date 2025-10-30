package cricinfo

import (
	"fmt"
	"io"

	"github.com/PuerkitoBio/goquery"
)

// MatchInfo captures high-level match metadata parsed from the scorecard page.
type MatchInfo struct {
	MatchID        int64
	Venue          string
	Toss           string
	BattingSession string
	BowlingSession string
}

// BattingRow represents a single batter line from the scorecard.
type BattingRow struct {
	PlayerName string
	Description string
	Runs int
	Balls int
	Minutes int
	Fours int
	Sixes int
	StrikeRate float32
	BattingPosition int
}

// BowlingRow represents a single bowler line from the scorecard.
type BowlingRow struct {
	PlayerName string
	Overs float32
	Balls int
	Maidens int
	Runs int
	Wickets int
	Dots int
	Fours int
	Sixes int
	Econ float32
	Wides int
	NoBalls int
}

// FieldingRow aggregates per-player fielding stats.
type FieldingRow struct {
	PlayerName string
	Catches int
	RunOuts int
	DroppedCatches int
	MissedRunOuts int
}

// SessionWeather contains weather-like attributes for a given session label.
type SessionWeather struct {
	Session string // e.g., "Morning", "Afternoon" or similar derived value
	Temp    *int
	Feels   *int
	Wind    *int
	Gust    *int
	Rain    *int
	Humidity *int
	Cloud   *int
	Pressure *int
	Viscosity *string
}

// ParseMatchPage parses a Cricinfo match scorecard HTML and returns structured data.
// NOTE: This is a stub parser; selectors will be filled to match HTML fixtures.
func ParseMatchPage(r io.Reader, matchID int64) (MatchInfo, []BattingRow, []BowlingRow, []FieldingRow, []SessionWeather, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil { return MatchInfo{}, nil, nil, nil, nil, err }
	_ = doc

	// TODO: implement robust selectors mirroring the Python prototype logic.
	mi := MatchInfo{MatchID: matchID}
	var batting []BattingRow
	var bowling []BowlingRow
	var fielding []FieldingRow
	var weather []SessionWeather

	return mi, batting, bowling, fielding, weather, nil
}

// ScorecardURL builds a legacy stats.espncricinfo URL for a given match id.
func ScorecardURL(matchID string) string {
	return fmt.Sprintf("https://stats.espncricinfo.com/ci/engine/match/%s.html", matchID)
}
