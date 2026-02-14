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
	dbMock := new(mocks.MockCricsheetDB)
	weatherMock := new(mocks.MockWeatherClient)

	// Arrange: inject mocks into package-level dependencies
	cricsheet.SetCricsheetDB(dbMock)
	cricsheet.SetWeatherClient(weatherMock)
	defer func() {
		cricsheet.SetCricsheetDB(new(mocks.MockCricsheetDB))
		cricsheet.SetWeatherClient(new(mocks.MockWeatherClient))
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

func TestImportMatchFile_InningTeamMismatch_Error(t *testing.T) {
	// Inning team name must match one of info.teams; otherwise otherTeam returns ""
	// and we would call GetOrCreateOpposition(""), corrupting the opposition table.
	ctx := context.Background()
	dbMock := new(mocks.MockCricsheetDB)
	weatherMock := new(mocks.MockWeatherClient)
	cricsheet.SetCricsheetDB(dbMock)
	cricsheet.SetWeatherClient(weatherMock)
	defer func() {
		cricsheet.SetCricsheetDB(new(mocks.MockCricsheetDB))
		cricsheet.SetWeatherClient(new(mocks.MockWeatherClient))
	}()

	// info.teams are Alpha, Beta but first inning has team "Gamma"
	bad := `{
      "info": {
        "balls_per_over": 6,
        "dates": ["2025-11-07"],
        "match_type": "T20",
        "teams": ["Alpha", "Beta"],
        "venue": "",
        "city": "",
        "season": "2025"
      },
      "innings": [
        {"team": "Gamma", "overs": [{"over": 0, "deliveries": [{"batter": "G1", "bowler": "G2", "non_striker": "G3", "runs": {"batter": 0, "extras": 0, "total": 0}}]}]}
      ]
    }`
	d := t.TempDir()
	file := writeJSON(t, d, "mismatch.json", bad)

	// Ingest runs UpsertMatch before the innings loop; allow it so we reach inning validation
	dbMock.On("UpsertMatch", mock.Anything, mock.MatchedBy(func(m *db.MatchInsert) bool { return m != nil })).Return(nil).Once()

	err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})
	require.Error(t, err)
	require.ErrorContains(t, err, "does not match match teams")
	require.ErrorContains(t, err, "Gamma")
	require.ErrorContains(t, err, "Alpha")
	require.ErrorContains(t, err, "Beta")
}

func TestImportMatchFile_InningEmptyTeamName_Error(t *testing.T) {
	// Inning with empty team name would make otherTeam return teamA; we validate batTeam non-empty first.
	ctx := context.Background()
	dbMock := new(mocks.MockCricsheetDB)
	weatherMock := new(mocks.MockWeatherClient)
	cricsheet.SetCricsheetDB(dbMock)
	cricsheet.SetWeatherClient(weatherMock)
	defer func() {
		cricsheet.SetCricsheetDB(new(mocks.MockCricsheetDB))
		cricsheet.SetWeatherClient(new(mocks.MockWeatherClient))
	}()

	bad := `{
      "info": {
        "balls_per_over": 6,
        "dates": ["2025-11-07"],
        "match_type": "T20",
        "teams": ["Alpha", "Beta"],
        "venue": "",
        "city": "",
        "season": "2025"
      },
      "innings": [
        {"team": "   ", "overs": [{"over": 0, "deliveries": [{"batter": "A1", "bowler": "B1", "non_striker": "A2", "runs": {"batter": 0, "extras": 0, "total": 0}}]}]}
      ]
    }`
	d := t.TempDir()
	file := writeJSON(t, d, "empty_team.json", bad)

	dbMock.On("UpsertMatch", mock.Anything, mock.MatchedBy(func(m *db.MatchInsert) bool { return m != nil })).Return(nil).Once()

	err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})
	require.Error(t, err)
	require.ErrorContains(t, err, "has no team name")
}

func TestImportMatchFile_BallsPerOverFallbackToSix(t *testing.T) {
	// Not parallel: uses package-level singletons via SetCricsheetDB/SetWeatherClient.
	ctx := context.Background()
	dbMock := new(mocks.MockCricsheetDB)
	weatherMock := new(mocks.MockWeatherClient)

	// Arrange
	cricsheet.SetCricsheetDB(dbMock)
	cricsheet.SetWeatherClient(weatherMock)
	defer func() {
		cricsheet.SetCricsheetDB(new(mocks.MockCricsheetDB))
		cricsheet.SetWeatherClient(new(mocks.MockWeatherClient))
	}()

	// stub recompute and fielding events to avoid touching real DB in unit tests
	cricsheet.SetRecomputeFn(func(_ context.Context, _ int64) error { return nil })
	cricsheet.SetInsertFieldingEventsBatchFn(func(_ context.Context, _ []db.FieldingEvent) error { return nil })

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

	// Expectations with typed matchers
	dbMock.On("UpsertMatch", ctx, mock.MatchedBy(func(m *db.MatchInsert) bool { return m != nil })).Return(nil)
	dbMock.On("UpsertMatchInning", ctx, mock.MatchedBy(func(mi *db.MatchInningInsert) bool { return mi != nil })).Return(nil)
	dbMock.On("UpsertBattingBatch", ctx, mock.MatchedBy(func(_ []db.Batting) bool { return true })).Return(nil)
	dbMock.On("UpsertBowlingBatch", ctx, mock.MatchedBy(func(_ []db.Bowling) bool { return true })).Return(nil)

	// Act
	_ = cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})

	// Assert
	dbMock.AssertExpectations(t)

	// Overs assertion: 7 legal balls at 6 balls/over => 1.1 notation
	calls := dbMock.Calls
	for _, call := range calls {
		if call.Method == "UpsertMatchInning" {
			args := call.Arguments
			mi := args.Get(1).(*db.MatchInningInsert)
			expected := float32(1.1)
			require.InDelta(t, expected, mi.OversBowled, 1e-6)
			return
		}
	}
	t.Fatalf("UpsertMatchInning was not called")
}
