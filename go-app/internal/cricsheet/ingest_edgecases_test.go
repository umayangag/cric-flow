package cricsheet_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
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
	// Not parallel: uses package-level singletons via SetCricsheetDB/SetWeatherClient.
	ctx := context.Background()
	dbMock := new(mocks.CricsheetDBMock)
	weatherMock := new(mocks.WeatherClientMock)

	// Arrange: inject mocks into package-level dependencies
	cricsheet.SetCricsheetDB(dbMock)
	cricsheet.SetWeatherClient(weatherMock)
	defer func() {
		// reset to fresh mocks with no expectations after test completes
		cricsheet.SetCricsheetDB(new(mocks.CricsheetDBMock))
		cricsheet.SetWeatherClient(new(mocks.WeatherClientMock))
	}()

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

	// Act
	err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

	// Assert
	require.Error(t, err)
	require.ErrorContains(t, err, "unsupported match_type")
	dbMock.AssertExpectations(t)
}

func TestImportMatchFile_BallsPerOverFallbackToSix(t *testing.T) {
	// Not parallel: uses package-level singletons via SetCricsheetDB/SetWeatherClient.
	ctx := context.Background()
	dbMock := new(mocks.CricsheetDBMock)
	weatherMock := new(mocks.WeatherClientMock)

	// Arrange
	cricsheet.SetCricsheetDB(dbMock)
	cricsheet.SetWeatherClient(weatherMock)
	defer func() {
		cricsheet.SetCricsheetDB(new(mocks.CricsheetDBMock))
		cricsheet.SetWeatherClient(new(mocks.WeatherClientMock))
	}()

	// stub recompute to avoid touching real DB in unit tests
	cricsheet.SetRecomputeFn(func(_ context.Context, _ int64) error { return nil })

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

	// Expectations
	dbMock.On("GetMatchFormatIDByCode", ctx, "T20").Return(int64(1), nil)
	dbMock.On("EnsureMatchWithFormat", ctx, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	dbMock.On("GetOrCreateSeason", ctx, "2025").Return(int64(200), nil)
	dbMock.On("UpdateMatchDetails", ctx, mock.Anything, mock.Anything).Return(nil)
	dbMock.On("GetOrCreateOpposition", ctx, mock.Anything).Return(int64(300), nil)
	dbMock.On("GetOrCreateByName", ctx, mock.Anything).Return(int64(0), nil)
	dbMock.On("UpsertBatting", ctx, mock.Anything).Return(nil)
	dbMock.On("UpsertBowling", ctx, mock.Anything).Return(nil)

	// Act
	_ = cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

	// Assert
	dbMock.AssertExpectations(t)

	// Overs assertion
	calls := dbMock.Calls
	for _, call := range calls {
		if call.Method == "UpdateMatchDetails" {
			args := call.Arguments
			upd := args.Get(2).(*db.MatchInfoUpdate)
			ov := upd.Overs
			require.NotNil(t, ov)
			expected := float32(1.1) // 7 legal balls at 6 balls/over => 1.1 notation
			require.InDelta(t, expected, *ov, 1e-6)
			return
		}
	}
	t.Fatalf("UpdateMatchDetails was not called")
}
