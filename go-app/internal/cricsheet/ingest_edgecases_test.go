package cricsheet_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-flow/go-app/internal/cricsheet"
	"github.com/umayangag/cric-flow/go-app/internal/cricsheet/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// spyTxForMatchInning implements db.CopyFromTx and captures OversBowled from match_inning upserts.
type spyTxForMatchInning struct {
	sawMatchInning bool
	oversBowled    *float32
}

func (t *spyTxForMatchInning) Exec(_ context.Context, sql string, args ...any) error {
	if strings.Contains(sql, "match_inning") && len(args) > 6 && t.oversBowled != nil {
		t.sawMatchInning = true
		switch v := args[6].(type) {
		case float32:
			*t.oversBowled = v
		case float64:
			*t.oversBowled = float32(v)
		}
	}
	return nil
}

func (t *spyTxForMatchInning) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nopRows{}, nil
}

func (t *spyTxForMatchInning) QueryRow(_ context.Context, _ string, _ ...any) db.Row {
	return nopRow{}
}

func (t *spyTxForMatchInning) CopyFrom(
	_ context.Context,
	_ pgx.Identifier,
	_ []string,
	_ pgx.CopyFromSource,
) (int64, error) {
	return 0, nil
}

func (t *spyTxForMatchInning) Commit(_ context.Context) error   { return nil }
func (t *spyTxForMatchInning) Rollback(_ context.Context) error { return nil }

type nopRows struct{}

func (nopRows) Next() bool          { return false }
func (nopRows) Scan(_ ...any) error { return nil }
func (nopRows) Close()              {}
func (nopRows) Err() error          { return nil }

type nopRow struct{}

func (nopRow) Scan(_ ...any) error { return nil }

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
	dbMock.On("UpsertMatch", mock.Anything, mock.MatchedBy(func(m *db.MatchInsert) bool { return m != nil })).
		Return(nil).
		Once()

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

	dbMock.On("UpsertMatch", mock.Anything, mock.MatchedBy(func(m *db.MatchInsert) bool { return m != nil })).
		Return(nil).
		Once()

	err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})
	require.Error(t, err)
	require.ErrorContains(t, err, "has no team name")
}

func TestImportMatchFile_BallsPerOverFallbackToSix(t *testing.T) {
	// Not parallel: uses package-level singletons and RunInTxFn.
	ctx := context.Background()

	// Spy tx to capture match_inning upsert args (OversBowled is 7th arg, 0-indexed: args[6])
	var oversBowled float32
	spyTx := &spyTxForMatchInning{oversBowled: &oversBowled}
	cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
		return inner(ctx, spyTx)
	})
	defer cricsheet.SetRunInTxFn(nil)

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

	// Act
	err := cricsheet.ImportMatchFile(ctx, file, &cricsheet.Options{})
	require.NoError(t, err)

	// Assert: 7 legal balls at 6 balls/over => 1.1 notation
	require.True(t, spyTx.sawMatchInning, "match_inning upsert was not called")
	require.InDelta(t, float32(1.1), oversBowled, 1e-6)
}
