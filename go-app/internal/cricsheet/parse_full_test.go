package cricsheet_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
)

// TestParse_FullShapes follows the gold standard: AAA with require assertions.
func TestParse_FullShapes(t *testing.T) {
	t.Parallel()

	// Arrange
	payload := map[string]any{
		"info": map[string]any{
			"balls_per_over": 6,
			"dates":          []string{"2025-11-07"},
			"match_type":     "T20",
			"teams":          []string{"India", "Australia"},
			"venue":          "Some Stadium",
			"city":           "Some City",
			"season":         "2025",
			"event":          map[string]any{"match_number": 1},
			"toss":           map[string]any{"winner": "India"},
			"outcome":        map[string]any{"winner": "India"},
		},
		"innings": []any{
			map[string]any{
				"team": "India",
				"overs": []any{
					map[string]any{
						"over": 0,
						"deliveries": []any{
							map[string]any{
								"batter":      "A",
								"bowler":      "X",
								"non_striker": "B",
								"runs":        map[string]any{"batter": 1, "extras": 0, "total": 1},
								"extras":      map[string]any{"wides": 0},
							},
							map[string]any{
								"batter":      "B",
								"bowler":      "X",
								"non_striker": "A",
								"runs":        map[string]any{"batter": 0, "extras": 1, "total": 1},
								"extras":      map[string]any{"wides": 1},
								"wickets": []any{
									map[string]any{
										"player_out": "B",
										"kind":       "bowled",
										"fielders":   []any{"F1", map[string]any{"name": "F2"}},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(payload)
	dec := json.NewDecoder(bytes.NewReader(b))

	// Act
	var m cricsheet.Match
	err := dec.Decode(&m)

	// Assert
	require.NoError(t, err)
	require.Len(t, m.Innings, 1)
	inn := m.Innings[0]
	require.Equal(t, "India", inn.Team)
	require.Len(t, inn.Overs, 1)
	require.Equal(t, 0, inn.Overs[0].Over)
	dels := inn.Overs[0].Deliveries
	require.Len(t, dels, 2)
	require.Equal(t, 1, dels[0].Runs.Total)
	require.Equal(t, 1, dels[1].Runs.Extras)
	require.NotNil(t, dels[1].Wickets)
	require.Len(t, *dels[1].Wickets, 1)
	w := (*dels[1].Wickets)[0]
	require.Equal(t, "B", w.PlayerOut)
	require.Equal(t, "bowled", w.Kind)
}
