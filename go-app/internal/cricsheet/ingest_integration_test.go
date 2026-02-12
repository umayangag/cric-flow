package cricsheet_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	tmocks "github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet/mocks"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// Note: Replaced test fakes with mockery-generated mocks (see internal/cricsheet/mocks).

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

// nop pool/tx implementations to satisfy db.PoolIface and db.CopyFromTx for offline tests
type nopPool struct{}

func (nopPool) Exec(_ context.Context, _ string, _ ...any) error { return nil }
func (nopPool) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nopRows{}, nil
}
func (nopPool) QueryRow(_ context.Context, _ string, _ ...any) db.Row { return nopRow{} }
func (nopPool) Begin(_ context.Context) (db.CopyFromTx, error)        { return nopTx{}, nil }

type nopTx struct{}

func (nopTx) Exec(_ context.Context, _ string, _ ...any) error { return nil }
func (nopTx) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nopRows{}, nil
}
func (nopTx) QueryRow(_ context.Context, _ string, _ ...any) db.Row { return nopRow{} }

func (nopTx) CopyFrom(
	_ context.Context,
	_ pgx.Identifier,
	_ []string,
	_ pgx.CopyFromSource,
) (int64, error) {
	return 0, nil
}
func (nopTx) Commit(_ context.Context) error   { return nil }
func (nopTx) Rollback(_ context.Context) error { return nil }

type nopRows struct{}

func (nopRows) Next() bool          { return false }
func (nopRows) Scan(_ ...any) error { return nil }
func (nopRows) Close()              {}
func (nopRows) Err() error          { return nil }

type nopRow struct{}

func (nopRow) Scan(_ ...any) error { return nil }

func TestImportMatchFile_OfflinePathsAndAggregates(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetCricsheetDB, SetWeatherClient).
	ctx := context.Background()
	// Provide a no-op pool so repository functions that require PoolAPI succeed in tests.
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })
	// Setup mocks to replace previous fakes
	mdb := &tmocks.CricsheetDBMock{}
	mweather := &tmocks.WeatherClientMock{}

	// Collections to assert behavior similar to earlier fakes
	var (
		updates      []*db.MatchInfoUpdate
		batting      []*db.Batting
		bowling      []*db.Bowling
		fielding     []*db.Fielding
		execs        []string
		weatherCalls int
		lastInnings  int
	)

	// DB expectations and behaviors
	mdb.On("GetMatchFormatIDByCode", mock.Anything, "T20").Return(int64(1), nil)
	mdb.On("EnsureMatchWithFormat", mock.Anything, mock.AnythingOfType("int64"), mock.AnythingOfType("int64"), mock.AnythingOfType("string")).
		Return(nil)
	mdb.On("GetOrCreateVenue", mock.Anything, mock.AnythingOfType("string")).Return(int64(1), nil)
	mdb.On("GetOrCreateSeason", mock.Anything, mock.AnythingOfType("string")).Return(int64(1), nil)
	mdb.On("GetOrCreateOpposition", mock.Anything, mock.AnythingOfType("string")).Return(int64(1), nil)
	mdb.On("GetOrCreateByName", mock.Anything, mock.AnythingOfType("string")).Return(int64(1), nil)
	mdb.On("UpdateMatchDetails", mock.Anything, mock.AnythingOfType("int64"), mock.AnythingOfType("*db.MatchInfoUpdate")).
		Return(nil).
		Run(func(args mock.Arguments) {
			upd := args.Get(2).(*db.MatchInfoUpdate)
			c := *upd
			updates = append(updates, &c)
		})
	mdb.On("UpsertBatting", mock.Anything, mock.AnythingOfType("*db.Batting")).
		Return(nil).
		Run(func(args mock.Arguments) {
			b := args.Get(1).(*db.Batting)
			c := *b
			batting = append(batting, &c)
		})
	mdb.On("UpsertBowling", mock.Anything, mock.AnythingOfType("*db.Bowling")).
		Return(nil).
		Run(func(args mock.Arguments) {
			b := args.Get(1).(*db.Bowling)
			c := *b
			bowling = append(bowling, &c)
		})
	mdb.On("UpsertFielding", mock.Anything, mock.AnythingOfType("*db.Fielding")).
		Return(nil).
		Run(func(args mock.Arguments) {
			f := args.Get(1).(*db.Fielding)
			c := *f
			fielding = append(fielding, &c)
		})
	mdb.On("Exec", mock.Anything, mock.AnythingOfType("string"), mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			sql := args.Get(1).(string)
			execs = append(execs, sql)
		})

	// Weather expectations
	mweather.On("EnqueueJob", mock.Anything, mock.AnythingOfType("int64"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("int")).
		Return(nil).
		Run(func(args mock.Arguments) {
			weatherCalls++
			lastInnings = args.Get(4).(int)
		})

	cricsheet.SetCricsheetDB(mdb)
	cricsheet.SetWeatherClient(mweather)
	// Avoid touching real DB recompute in tests
	cricsheet.SetRecomputeFn(func(_ context.Context, _ int64) error { return nil })
	defer func() {
		cricsheet.SetCricsheetDB(&tmocks.CricsheetDBMock{})
		cricsheet.SetWeatherClient(&tmocks.WeatherClientMock{})
		cricsheet.SetRecomputeFn(func(_ context.Context, _ int64) error { return nil })
	}()

	d := t.TempDir()
	file := writeTempJSON(t, d, "a.json", sampleJSON)

	opts := &cricsheet.Options{PlaceholdersWeather: true, PlaceholdersFielding: true, WeatherEnqueue: true}
	err := cricsheet.ImportMatchFile(ctx, file, opts)
	require.NoError(t, err)
	// Expect two UpdateMatchDetails (two innings)
	require.Equal(t, 2, len(updates), "expected 2 updates")
	// Target for second innings should equal first innings total (4 + 1 + 6 = 11)
	require.NotNil(t, updates[1].Target)
	require.Equal(t, 11, *updates[1].Target)
	// Expect at least one batting and bowling record upserted
	require.NotEmpty(t, batting)
	require.NotEmpty(t, bowling)
	// Fielding placeholders should be created for seen players (>= players seen)
	require.NotEmpty(t, fielding)
	// Weather placeholders executed and enqueue called
	require.NotEmpty(t, execs)
	require.NotZero(t, weatherCalls)
	require.Equal(t, 2, lastInnings)
}

func TestImportDir_SortsAndCountsJSON(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetCricsheetDB, SetWeatherClient).
	ctx := context.Background()
	// Provide a no-op pool so repository functions that require PoolAPI succeed in tests.
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })
	mdb := &tmocks.CricsheetDBMock{}
	mweather := &tmocks.WeatherClientMock{}
	// DB expectations minimal for directory import
	mdb.On("GetMatchFormatIDByCode", mock.Anything, mock.AnythingOfType("string")).Return(int64(1), nil)
	mdb.On("EnsureMatchWithFormat", mock.Anything, mock.AnythingOfType("int64"), mock.AnythingOfType("int64"), mock.AnythingOfType("string")).
		Return(nil)
	mdb.On("GetOrCreateVenue", mock.Anything, mock.AnythingOfType("string")).Return(int64(1), nil)
	mdb.On("GetOrCreateSeason", mock.Anything, mock.AnythingOfType("string")).Return(int64(1), nil)
	mdb.On("GetOrCreateOpposition", mock.Anything, mock.AnythingOfType("string")).Return(int64(1), nil)
	mdb.On("GetOrCreateByName", mock.Anything, mock.AnythingOfType("string")).Return(int64(1), nil)
	mdb.On("UpdateMatchDetails", mock.Anything, mock.AnythingOfType("int64"), mock.AnythingOfType("*db.MatchInfoUpdate")).
		Return(nil)
	mdb.On("UpsertBatting", mock.Anything, mock.AnythingOfType("*db.Batting")).Return(nil)
	mdb.On("UpsertBowling", mock.Anything, mock.AnythingOfType("*db.Bowling")).Return(nil)
	mdb.On("UpsertFielding", mock.Anything, mock.AnythingOfType("*db.Fielding")).Return(nil)
	mdb.On("Exec", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil)
	mweather.On("EnqueueJob", mock.Anything, mock.AnythingOfType("int64"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("int")).
		Return(nil)
	cricsheet.SetCricsheetDB(mdb)
	cricsheet.SetWeatherClient(mweather)
	cricsheet.SetRecomputeFn(func(_ context.Context, _ int64) error { return nil })
	defer func() {
		cricsheet.SetCricsheetDB(&tmocks.CricsheetDBMock{})
		cricsheet.SetWeatherClient(&tmocks.WeatherClientMock{})
		cricsheet.SetRecomputeFn(func(_ context.Context, _ int64) error { return nil })
	}()

	d := t.TempDir()
	_ = writeTempJSON(t, d, "b.json", sampleJSON)
	_ = writeTempJSON(t, d, "a.json", sampleJSON)

	cnt, err := cricsheet.ImportDir(ctx, d, &cricsheet.Options{})
	require.NoError(t, err)
	require.Equal(t, 2, cnt)
}
