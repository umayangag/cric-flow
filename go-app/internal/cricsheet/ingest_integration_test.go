package cricsheet_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/mock"
)

// minimal two-innings JSON exercising wickets/fielders, wides/no-balls, and runs
const sampleJSON = `{
  "info": {
    "balls_per_over": 6,
    "dates": ["2024-01-02"],
    "match_type": "T20",
    "teams": ["Alpha", "Beta"],
    "venue": "The Oval",
    "city": "Metropolis",
    "season": "2024",
    "event": {"match_number": 12},
    "toss": {"winner": "Alpha"},
    "outcome": {"winner": "Alpha"}
  },
  "innings": [
    {"team":"Alpha","overs":[
      {"over":1,"deliveries":[
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":4,"extras":0,"total":4}},
        {"batter":"A1","bowler":"B1","non_striker":"A2","runs":{"batter":0,"extras":1,"total":1},"extras":{"wides":1}},
        {"batter":"A2","bowler":"B1","non_striker":"A1","runs":{"batter":6,"extras":0,"total":6}}
      ]}
    ]},
    {"team":"Beta","overs":[
      {"over":1,"deliveries":[
        {"batter":"B1","bowler":"A1","non_striker":"B2","runs":{"batter":0,"extras":0,"total":0},"wickets":[{"player_out":"B1","kind":"caught","fielders":[{"name":"A3"}]}]},
        {"batter":"B2","bowler":"A1","non_striker":"B3","runs":{"batter":1,"extras":0,"total":1}},
        {"batter":"B3","bowler":"A1","non_striker":"B2","runs":{"batter":0,"extras":0,"total":0}}
      ]}
    ]}
  ]
}`

func writeTempJSON(t *testing.T, dir string, name string, data string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatalf("write temp json: %v", err)
	}
	return p
}

func TestImportMatchFile_Integration(t *testing.T) {
	ctx := context.Background()
	dbMock := new(CricsheetDBMock)
	weatherMock := new(WeatherClientMock)

	// Set up mocks
	prevDB := cricDB
	prevW := weatherClient
	SetCricsheetDB(dbMock)
	SetWeatherClient(weatherMock)
	defer func() {
		SetCricsheetDB(prevDB)
		SetWeatherClient(prevW)
	}()

	d := t.TempDir()
	file := writeTempJSON(t, d, "a.json", sampleJSON)

	// Expectations
	dbMock.On("GetMatchFormatIDByCode", ctx, "T20").Return(int64(1), nil)
	dbMock.On("EnsureMatchWithFormat", ctx, mock.Anything, mock.Anything).Return(nil)
	dbMock.On("GetOrCreateVenue", ctx, "The Oval").Return(int64(100), nil)
	dbMock.On("GetOrCreateSeason", ctx, "2024").Return(int64(200), nil)
	dbMock.On("UpdateMatchDetails", ctx, mock.Anything, mock.Anything).Return(nil)
	dbMock.On("GetOrCreateOpposition", ctx, mock.Anything).Return(int64(300), nil)
	dbMock.On("GetOrCreateByName", ctx, mock.Anything).Return(int64(0), nil)
	dbMock.On("UpsertBatting", ctx, mock.Anything).Return(nil)
	dbMock.On("UpsertBowling", ctx, mock.Anything).Return(nil)
	dbMock.On("UpsertFielding", ctx, mock.Anything).Return(nil)
	dbMock.On("Exec", ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	dbMock.On("RecomputeFieldingAggregates", ctx, mock.Anything).Return(nil)
	weatherMock.On("EnqueueJob", ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	opts := &Options{PlaceholdersWeather: true, PlaceholdersFielding: true, WeatherEnqueue: true}
	if err := ImportMatchFile(ctx, file, opts); err != nil {
		t.Fatalf("ImportMatchFile error: %v", err)
	}

	// Assertions
	dbMock.AssertExpectations(t)
	weatherMock.AssertExpectations(t)
}

func TestImportDir_SortsAndCountsJSON(t *testing.T) {
	ctx := context.Background()
	dbMock := new(CricsheetDBMock)
	weatherMock := new(WeatherClientMock)

	// Set up mocks
	prevDB := cricDB
	prevW := weatherClient
	SetCricsheetDB(dbMock)
	SetWeatherClient(weatherMock)
	defer func() {
		SetCricsheetDB(prevDB)
		SetWeatherClient(prevW)
	}()

	d := t.TempDir()
	_ = writeTempJSON(t, d, "b.json", sampleJSON)
	_ = writeTempJSON(t, d, "a.json", sampleJSON)

	// Expectations
	dbMock.On("GetMatchFormatIDByCode", ctx, "T20").Return(int64(1), nil)
	dbMock.On("EnsureMatchWithFormat", ctx, mock.Anything, mock.Anything).Return(nil)
	dbMock.On("GetOrCreateVenue", ctx, "The Oval").Return(int64(100), nil)
	dbMock.On("GetOrCreateSeason", ctx, "2024").Return(int64(200), nil)
	dbMock.On("UpdateMatchDetails", ctx, mock.Anything, mock.Anything).Return(nil)
	dbMock.On("GetOrCreateOpposition", ctx, mock.Anything).Return(int64(300), nil)
	dbMock.On("GetOrCreateByName", ctx, mock.Anything).Return(int64(0), nil)
	dbMock.On("UpsertBatting", ctx, mock.Anything).Return(nil)
	dbMock.On("UpsertBowling", ctx, mock.Anything).Return(nil)
	dbMock.On("RecomputeFieldingAggregates", ctx, mock.Anything).Return(nil)

	cnt, err := ImportDir(ctx, d, &Options{})
	if err != nil {
		t.Fatalf("ImportDir error: %v", err)
	}
	if cnt != 2 {
		t.Fatalf("expected 2 files imported, got %d", cnt)
	}

	// Assertions
	dbMock.AssertExpectations(t)
}
