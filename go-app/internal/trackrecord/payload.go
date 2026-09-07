package trackrecord

import (
	"encoding/json"
	"fmt"
)

// storedPayload is the part of a served answer the record reads back: the sides as they
// were named, the players each eleven named, and the scorecard's served ranges. It is a
// projection of predictteam.Result read from the stored bytes rather than the type itself,
// because the record must keep reading rows stored under an earlier shape of the answer --
// a field the payload lacks is simply absent here, never a decode error.
type storedPayload struct {
	Team1Side struct {
		DisplayName string `json:"display_name"`
	} `json:"team1_side"`
	Team2Side struct {
		DisplayName string `json:"display_name"`
	} `json:"team2_side"`
	Team1     []storedPlayer `json:"team1"`
	Team2     []storedPlayer `json:"team2"`
	Scorecard *struct {
		// SharedFactor is absent on answers served before P2-4; the column decides the
		// population and this is not read for it (the column is nil for those rows too).
		Team1Innings storedInnings `json:"team1_innings"`
		Team2Innings storedInnings `json:"team2_innings"`
	} `json:"scorecard"`
}

type storedPlayer struct {
	PlayerID int64 `json:"player_id"`
}

type storedInnings struct {
	P10 float64 `json:"p10"`
	P90 float64 `json:"p90"`
}

// readPayload decodes one stored answer. The record never refuses a row over its payload:
// the caller keeps the row in its state and reports the error on it (§8.7).
func readPayload(raw []byte) (*storedPayload, error) {
	var payload storedPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("read stored payload: %w", err)
	}
	return &payload, nil
}

// playerIDs is the ids one eleven named.
func playerIDs(players []storedPlayer) []int64 {
	ids := make([]int64, 0, len(players))
	for i := range players {
		ids = append(ids, players[i].PlayerID)
	}
	return ids
}
