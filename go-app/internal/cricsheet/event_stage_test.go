package cricsheet_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
)

// The two event fields X-3 added to the importer. Cricsheet spells a pool as a string in
// most files and as a bare number in the rest, so the decode has to accept both without
// the importer having to know which kind of file it is holding.
func TestEvent_UnmarshalJSON_CarriesStageAndGroup(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		in          string
		wantName    string
		wantStage   string
		wantGroup   string
		wantNumber  *int
		wantNoEvent bool
	}{
		{
			name:      "stage and string group",
			in:        `{"name":"Vitality Blast","stage":"Semi Final","group":"North"}`,
			wantName:  "Vitality Blast",
			wantStage: "Semi Final",
			wantGroup: "North",
		},
		{
			name:      "numeric group",
			in:        `{"name":"Super Smash","group":1}`,
			wantName:  "Super Smash",
			wantGroup: "1",
		},
		{
			name:       "match number without stage or group",
			in:         `{"name":"Indian Premier League","match_number":42}`,
			wantName:   "Indian Premier League",
			wantNumber: intPointer(42),
		},
		{
			name:     "null stage and group",
			in:       `{"name":"One-Day Cup","stage":null,"group":null}`,
			wantName: "One-Day Cup",
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var event cricsheet.Event

			err := json.Unmarshal([]byte(tc.in), &event)

			require.NoError(t, err)
			require.Equal(t, tc.wantName, event.Name)
			require.Equal(t, tc.wantStage, event.Stage)
			require.Equal(t, tc.wantGroup, string(event.Group))
			require.Equal(t, tc.wantNumber, event.MatchNumber)
		})
	}
}

func TestInfo_UnmarshalJSON_EventFieldsReachTheInfo(t *testing.T) {
	t.Parallel()

	var info cricsheet.Info

	err := json.Unmarshal([]byte(`{
		"match_type":"T20",
		"dates":["2019-05-12"],
		"teams":["A","B"],
		"event":{"name":"Indian Premier League","stage":"Final"}
	}`), &info)

	require.NoError(t, err)
	require.NotNil(t, info.Event)
	require.Equal(t, "Final", info.Event.Stage)
	require.Empty(t, string(info.Event.Group))
}

func intPointer(value int) *int {
	return &value
}
