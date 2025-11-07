package cricsheet

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper: write a temp JSON file
func writeJSON(t *testing.T, dir, name, data string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatalf("write json: %v", err)
	}
	return p
}

func TestImportMatchFile_UnknownMatchType_Error(t *testing.T) {
	ctx := context.Background()
	prevDB := cricDB
	prevW := weatherClient
	fdb := newFakeDB()
	SetCricsheetDB(fdb)
	SetWeatherClient(&fakeWeather{})
	defer func() { SetCricsheetDB(prevDB); SetWeatherClient(prevW) }()

	// Minimal JSON with unsupported match_type
	bad := `{
	  "info": {
	    "balls_per_over": 6,
	    "dates": ["2025-11-07"],
	    "match_type": "Friendly",
	    "teams": ["Alpha", "Beta"],
	    "venue": "",
	    "city": "",
	    "season": "2025"
	  },
	  "innings": []
	}`
	d := t.TempDir()
	file := writeJSON(t, d, "bad.json", bad)

	err := ImportMatchFile(ctx, file, &Options{})
	if err == nil {
		t.Fatalf("expected error for unknown match_type")
	}
	if !strings.Contains(err.Error(), "unsupported match_type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportMatchFile_BallsPerOverFallbackToSix(t *testing.T) {
	ctx := context.Background()
	prevDB := cricDB
	prevW := weatherClient
	fdb := newFakeDB()
	SetCricsheetDB(fdb)
	SetWeatherClient(&fakeWeather{})
	defer func() { SetCricsheetDB(prevDB); SetWeatherClient(prevW) }()

	// balls_per_over is 0 -> should fallback to 6
	// Create 7 legal deliveries so overs should be 1.1 (i.e., 1 over + 1 ball)
	// Keep it minimal: one innings, one over block with 7 deliveries (we can emulate two overs with over index duplication; ingestion aggregates by counting legal balls only).
	good := `{
	  "info": {
	    "balls_per_over": 0,
	    "dates": ["2025-11-07"],
	    "match_type": "T20",
	    "teams": ["Alpha", "Beta"],
	    "venue": "",
	    "city": "",
	    "season": "2025"
	  },
	  "innings": [
	    {"team":"Alpha","overs":[
	      {"over":1,"deliveries":[
	        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}},
	        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}},
	        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}},
	        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}},
	        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}},
	        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}},
	        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":0,"total":0}}
	      ]}
	    ]}
	  ]
	}`
	d := t.TempDir()
	file := writeJSON(t, d, "good.json", good)

	if err := ImportMatchFile(ctx, file, &Options{}); err != nil {
		t.Fatalf("ImportMatchFile error: %v", err)
	}
	if len(fdb.updates) == 0 {
		t.Fatalf("expected at least one UpdateMatchDetails call")
	}
	ov := fdb.updates[0].Overs
	if ov == nil {
		t.Fatalf("expected Overs to be set")
	}
	expected := float32(1.1) // 7 legal balls at 6 balls/over => 1.1 notation
	if math.Abs(float64(*ov)-float64(expected)) > 1e-6 {
		t.Fatalf("overs mismatch: got %.3f want %.3f", *ov, expected)
	}
}
