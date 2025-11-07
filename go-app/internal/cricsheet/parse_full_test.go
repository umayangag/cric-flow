package cricsheet

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestParse_FullShapes(t *testing.T) {
	// Construct a cricsheet-like JSON with overs, deliveries, extras, and wickets
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
	var m Match
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(m.Innings) != 1 {
		t.Fatalf("want 1 innings, got %d", len(m.Innings))
	}
	inn := m.Innings[0]
	if inn.Team != "India" {
		t.Fatalf("team mismatch: %s", inn.Team)
	}
	if len(inn.Overs) != 1 || inn.Overs[0].Over != 0 {
		t.Fatalf("expected one over #0, got: %+v", inn.Overs)
	}
	dels := inn.Overs[0].Deliveries
	if len(dels) != 2 {
		t.Fatalf("expected 2 deliveries, got %d", len(dels))
	}
	if dels[0].Runs.Total != 1 || dels[1].Runs.Extras != 1 {
		t.Fatalf("unexpected runs parsed: %+v", dels)
	}
	if dels[1].Wickets == nil || len(*dels[1].Wickets) != 1 {
		t.Fatalf("expected one wicket, got: %+v", dels[1].Wickets)
	}
	// Ensure fielders collection decoded properly (two names extracted)
	w := (*dels[1].Wickets)[0]
	if w.PlayerOut != "B" || w.Kind != "bowled" {
		t.Fatalf("wicket parsed incorrectly: %+v", w)
	}
}
