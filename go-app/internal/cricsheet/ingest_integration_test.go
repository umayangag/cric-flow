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
	tmocks "github.com/umayangag/cric-flow/go-app/internal/cricsheet/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/db"
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

// offlineSpyTx captures match_inning upserts, CopyFrom table names, and execs for integration-style assertions.
type offlineSpyTx struct {
	innings                                         []db.MatchInningInsert
	execs                                           []string
	sawBattingCopy, sawBowlingCopy, sawFieldingCopy bool
}

// Column indexes for match_inning INSERT in db.UpsertMatchInningTx (repo_match.go).
// Update these if the INSERT column order changes.
const (
	matchInningArgMatchID                 = 0
	matchInningArgInningNumber            = 1
	matchInningArgBattingTeamOppositionID = 2
	matchInningArgBowlingTeamOppositionID = 3
	matchInningArgRunsScored              = 4
	matchInningArgWicketsLost             = 5
	matchInningArgOversBowled             = 6
	matchInningArgBallsBowled             = 7
	matchInningArgRunRate                 = 8
	matchInningArgTargetRuns              = 9
	matchInningArgExtras                  = 10
	matchInningArgWinnerOppositionID      = 11
	matchInningArgCount                   = 12
)

func (t *offlineSpyTx) Exec(_ context.Context, sql string, args ...any) error {
	t.execs = append(t.execs, sql)
	if strings.Contains(sql, "match_inning") && strings.Contains(sql, "INSERT") && len(args) >= matchInningArgCount {
		mi := db.MatchInningInsert{
			MatchID:                 toInt64(args[matchInningArgMatchID]),
			InningNumber:            toInt(args[matchInningArgInningNumber]),
			BattingTeamOppositionID: toInt64(args[matchInningArgBattingTeamOppositionID]),
			BowlingTeamOppositionID: toInt64(args[matchInningArgBowlingTeamOppositionID]),
			RunsScored:              toInt(args[matchInningArgRunsScored]),
			WicketsLost:             toInt(args[matchInningArgWicketsLost]),
			OversBowled:             toFloat32(args[matchInningArgOversBowled]),
			BallsBowled:             toInt(args[matchInningArgBallsBowled]),
			RunRate:                 toFloat32Ptr(args[matchInningArgRunRate]),
			TargetRuns:              toIntPtr(args[matchInningArgTargetRuns]),
			Extras:                  toInt(args[matchInningArgExtras]),
			WinnerOppositionID:      toInt64Ptr(args[matchInningArgWinnerOppositionID]),
		}
		t.innings = append(t.innings, mi)
	}
	return nil
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	default:
		return 0
	}
}

func toInt64Ptr(v any) *int64 {
	if v == nil {
		return nil
	}
	if p, ok := v.(*int64); ok {
		return p
	}
	n := toInt64(v)
	return &n
}

func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	default:
		return 0
	}
}

func toIntPtr(v any) *int {
	if v == nil {
		return nil
	}
	if p, ok := v.(*int); ok {
		return p
	}
	n := toInt(v)
	return &n
}

func toFloat32(v any) float32 {
	switch x := v.(type) {
	case float32:
		return x
	case float64:
		return float32(x)
	default:
		return 0
	}
}

func toFloat32Ptr(v any) *float32 {
	if v == nil {
		return nil
	}
	if p, ok := v.(*float32); ok {
		return p
	}
	f := toFloat32(v)
	return &f
}

func (t *offlineSpyTx) Query(_ context.Context, _ string, _ ...any) (db.Rows, error) {
	return nopRows{}, nil
}

func (t *offlineSpyTx) QueryRow(_ context.Context, _ string, _ ...any) db.Row {
	return nopRow{}
}

func (t *offlineSpyTx) CopyFrom(
	_ context.Context,
	table pgx.Identifier,
	_ []string,
	src pgx.CopyFromSource,
) (int64, error) {
	tbl := strings.Join(table, ".")
	if strings.Contains(tbl, "batting") {
		t.sawBattingCopy = true
	}
	if strings.Contains(tbl, "bowling") {
		t.sawBowlingCopy = true
	}
	if strings.Contains(tbl, "fielding") {
		t.sawFieldingCopy = true
	}
	n := int64(0)
	for src.Next() {
		n++
	}
	return n, src.Err()
}

func (t *offlineSpyTx) Commit(_ context.Context) error   { return nil }
func (t *offlineSpyTx) Rollback(_ context.Context) error { return nil }

func TestImportMatchFile_OfflinePathsAndAggregates(t *testing.T) {
	// Not parallel: uses package-level singletons (db.PoolAPI, SetRunInTxFn, SetWeatherClient).
	ctx := context.Background()
	// Provide a no-op pool so cache lookups that use PoolAPI succeed.
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })

	// Spy tx to capture match_inning upserts, batting/bowling/fielding batches, and execs
	spy := &offlineSpyTx{}
	cricsheet.SetRunInTxFn(func(ctx context.Context, inner func(context.Context, db.CopyFromTx) error) error {
		return inner(ctx, spy)
	})
	t.Cleanup(func() { cricsheet.SetRunInTxFn(nil) })

	mweather := &tmocks.MockWeatherClient{}
	weatherCalls := 0
	lastInnings := 0
	mweather.On("EnqueueJob", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			weatherCalls++
			lastInnings = args.Get(4).(int)
		})

	cricsheet.SetWeatherClient(mweather)
	t.Cleanup(func() {
		cricsheet.SetWeatherClient(&tmocks.MockWeatherClient{})
	})

	d := t.TempDir()
	file := writeTempJSON(t, d, "a.json", sampleJSON)

	opts := &cricsheet.Options{PlaceholdersWeather: true, PlaceholdersFielding: true, WeatherEnqueue: true}
	err := cricsheet.ImportMatchFile(ctx, file, opts)
	require.NoError(t, err)

	// Expect two match_inning upserts (two innings)
	require.Len(t, spy.innings, 2, "expected 2 inning upserts")
	// Target for second innings should equal first innings total (4 + 1 + 6 = 11)
	require.NotNil(t, spy.innings[1].TargetRuns)
	require.Equal(t, 11, *spy.innings[1].TargetRuns)
	// Expect CopyFrom for batting, bowling, fielding (proves those code paths ran)
	require.True(t, spy.sawBattingCopy, "expected batting CopyFrom")
	require.True(t, spy.sawBowlingCopy, "expected bowling CopyFrom")
	require.True(t, spy.sawFieldingCopy, "expected fielding CopyFrom")
	// Weather placeholders executed and enqueue called
	require.NotEmpty(t, spy.execs)
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
	mdb := &tmocks.MockCricsheetDB{}
	mweather := &tmocks.MockWeatherClient{}
	anyCtx := mock.MatchedBy(func(c context.Context) bool { return c != nil })
	// DB expectations minimal for directory import (typed matchers)
	mdb.On("GetMatchFormatIDByCode", anyCtx, mock.MatchedBy(func(_ string) bool { return true })).Return(int64(1), nil)
	mdb.On("GetOrCreateVenue", anyCtx, mock.MatchedBy(func(_ string) bool { return true })).Return(int64(1), nil)
	mdb.On("GetOrCreateSeason", anyCtx, mock.MatchedBy(func(_ string) bool { return true })).Return(int64(1), nil)
	mdb.On("GetOrCreateOpposition", anyCtx, mock.MatchedBy(func(_ string) bool { return true })).Return(int64(1), nil)
	mdb.On("GetOrCreateByName", anyCtx, mock.MatchedBy(func(_ string) bool { return true })).Return(int64(1), nil)
	mdb.On("UpsertMatch", anyCtx, mock.MatchedBy(func(m *db.MatchInsert) bool { return m != nil })).Return(nil)
	mdb.On("UpsertMatchInning", anyCtx, mock.MatchedBy(func(mi *db.MatchInningInsert) bool { return mi != nil })).
		Return(nil)
	mdb.On("UpsertBattingBatch", anyCtx, mock.MatchedBy(func(_ []db.Batting) bool { return true })).Return(nil)
	mdb.On("UpsertBowlingBatch", anyCtx, mock.MatchedBy(func(_ []db.Bowling) bool { return true })).Return(nil)
	mdb.On("UpsertFieldingBatch", anyCtx, mock.MatchedBy(func(_ []db.Fielding) bool { return true })).Return(nil)
	mdb.On("Exec", anyCtx, mock.MatchedBy(func(_ string) bool { return true }), mock.MatchedBy(func(_ any) bool { return true })).
		Return(nil)
	mweather.On("EnqueueJob", anyCtx, mock.MatchedBy(func(_ int64) bool { return true }), mock.MatchedBy(func(_ string) bool { return true }), mock.MatchedBy(func(_ string) bool { return true }), mock.MatchedBy(func(_ int) bool { return true })).
		Return(nil)
	cricsheet.SetCricsheetDB(mdb)
	cricsheet.SetWeatherClient(mweather)
	defer func() {
		cricsheet.SetCricsheetDB(&tmocks.MockCricsheetDB{})
		cricsheet.SetWeatherClient(&tmocks.MockWeatherClient{})
	}()

	d := t.TempDir()
	_ = writeTempJSON(t, d, "b.json", sampleJSON)
	_ = writeTempJSON(t, d, "a.json", sampleJSON)

	cnt, err := cricsheet.ImportDir(ctx, d, &cricsheet.Options{}, 1)
	require.NoError(t, err)
	require.Equal(t, 2, cnt)
}
