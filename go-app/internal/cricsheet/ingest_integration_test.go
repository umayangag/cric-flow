package cricsheet_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

type fakeDB struct {
	formatsByCode map[string]int64
	venues        map[string]int64
	seasons       map[string]int64
	players       map[string]int64
	oppos         map[string]int64
	batting       []*db.Batting
	bowling       []*db.Bowling
	fielding      []*db.Fielding
	updates       []*db.MatchInfoUpdate
	execs         []string
}

func newFakeDB() *fakeDB {
	return &fakeDB{
		formatsByCode: map[string]int64{"T20": 1, "T20I": 2, "ODI": 3, "TEST": 4},
		venues:        map[string]int64{},
		seasons:       map[string]int64{},
		players:       map[string]int64{},
		oppos:         map[string]int64{},
	}
}

func (f *fakeDB) GetMatchFormatIDByCode(_ context.Context, code string) (int64, error) {
	if id, ok := f.formatsByCode[code]; ok {
		return id, nil
	}
	return 0, nil
}

func (f *fakeDB) EnsureMatchWithFormat(_ context.Context, _ int64, _ int64) error {
	return nil
}

func (f *fakeDB) GetOrCreateVenue(_ context.Context, name string) (int64, error) {
	if id, ok := f.venues[name]; ok {
		return id, nil
	}
	id := int64(len(f.venues) + 1)
	f.venues[name] = id
	return id, nil
}

func (f *fakeDB) GetOrCreateSeason(_ context.Context, name string) (int64, error) {
	if id, ok := f.seasons[name]; ok {
		return id, nil
	}
	id := int64(len(f.seasons) + 1)
	f.seasons[name] = id
	return id, nil
}

func (f *fakeDB) GetOrCreateOpposition(_ context.Context, name string) (int64, error) {
	if id, ok := f.oppos[name]; ok {
		return id, nil
	}
	id := int64(len(f.oppos) + 1)
	f.oppos[name] = id
	return id, nil
}

func (f *fakeDB) UpdateMatchDetails(_ context.Context, _ int64, upd *db.MatchInfoUpdate) error {
	// Copy values to avoid mutation surprises
	u := *upd
	f.updates = append(f.updates, &u)
	return nil
}

func (f *fakeDB) GetOrCreateByName(_ context.Context, name string) (int64, error) {
	if id, ok := f.players[name]; ok {
		return id, nil
	}
	id := int64(len(f.players) + 1)
	f.players[name] = id
	return id, nil
}

func (f *fakeDB) UpsertBatting(_ context.Context, b *db.Batting) error {
	bb := *b
	f.batting = append(f.batting, &bb)
	return nil
}

func (f *fakeDB) UpsertBowling(_ context.Context, b *db.Bowling) error {
	bb := *b
	f.bowling = append(f.bowling, &bb)
	return nil
}

func (f *fakeDB) UpsertFielding(_ context.Context, ff *db.Fielding) error {
	cop := *ff
	f.fielding = append(f.fielding, &cop)
	return nil
}

func (f *fakeDB) Exec(_ context.Context, sql string, _ ...any) error {
	f.execs = append(f.execs, sql)
	return nil
}

type fakeWeather struct {
	calls int
	last  struct {
		matchID     int64
		city, venue string
		innings     int
	}
}

func (f *fakeWeather) EnqueueJob(_ context.Context, matchID int64, city, venue string, innings int) error {
	f.calls++
	f.last.matchID = matchID
	f.last.city = city
	f.last.venue = venue
	f.last.innings = innings
	return nil
}

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

type nopRow struct{}

func (nopRow) Scan(_ ...any) error { return nil }

func TestImportMatchFile_OfflinePathsAndAggregates(t *testing.T) {
	ctx := context.Background()
	// Provide a no-op pool so repository functions that require PoolAPI succeed in tests.
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })
	fdb := newFakeDB()
	fw := &fakeWeather{}
	cricsheet.SetCricsheetDB(fdb)
	cricsheet.SetWeatherClient(fw)
	// Avoid touching real DB recompute in tests
	cricsheet.SetRecomputeFn(func(_ context.Context, _ int64) error { return nil })
	defer func() {
		cricsheet.SetCricsheetDB(newFakeDB())
		cricsheet.SetWeatherClient(&fakeWeather{})
		cricsheet.SetRecomputeFn(func(_ context.Context, _ int64) error { return nil })
	}()

	d := t.TempDir()
	file := writeTempJSON(t, d, "a.json", sampleJSON)

	opts := &cricsheet.Options{PlaceholdersWeather: true, PlaceholdersFielding: true, WeatherEnqueue: true}
	if err := cricsheet.ImportMatchFile(ctx, file, opts); err != nil {
		t.Fatalf("ImportMatchFile error: %v", err)
	}
	// Expect two UpdateMatchDetails (two innings)
	if len(fdb.updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(fdb.updates))
	}
	// Target for second innings should equal first innings total (4 + 1 + 6 = 11)
	if fdb.updates[1].Target == nil || *fdb.updates[1].Target != 11 {
		t.Fatalf("second innings target mismatch, got %+v", fdb.updates[1].Target)
	}
	// Expect at least one batting and bowling record upserted
	if len(fdb.batting) == 0 || len(fdb.bowling) == 0 {
		t.Fatalf("expected batting and bowling upserts, got batting=%d bowling=%d", len(fdb.batting), len(fdb.bowling))
	}
	// Fielding placeholders should be created for seen players (>= players seen)
	if len(fdb.fielding) == 0 {
		t.Fatalf("expected placeholder fielding upserts, got 0")
	}
	// Weather placeholders executed and enqueue called
	if len(fdb.execs) == 0 {
		t.Fatalf("expected at least one Exec for weather placeholders")
	}
	if fw.calls == 0 || fw.last.innings != 2 {
		t.Fatalf("expected weather enqueue once with innings=2, got calls=%d last=%+v", fw.calls, fw.last)
	}
}

func TestImportDir_SortsAndCountsJSON(t *testing.T) {
	ctx := context.Background()
	// Provide a no-op pool so repository functions that require PoolAPI succeed in tests.
	prevPool := db.PoolAPI
	db.SetPoolAPI(nopPool{})
	t.Cleanup(func() { db.SetPoolAPI(prevPool) })
	fdb := newFakeDB()
	fw := &fakeWeather{}
	cricsheet.SetCricsheetDB(fdb)
	cricsheet.SetWeatherClient(fw)
	cricsheet.SetRecomputeFn(func(_ context.Context, _ int64) error { return nil })
	defer func() {
		cricsheet.SetCricsheetDB(newFakeDB())
		cricsheet.SetWeatherClient(&fakeWeather{})
		cricsheet.SetRecomputeFn(func(_ context.Context, _ int64) error { return nil })
	}()

	d := t.TempDir()
	_ = writeTempJSON(t, d, "b.json", sampleJSON)
	_ = writeTempJSON(t, d, "a.json", sampleJSON)

	cnt, err := cricsheet.ImportDir(ctx, d, &cricsheet.Options{})
	if err != nil {
		t.Fatalf("ImportDir error: %v", err)
	}
	if cnt != 2 {
		t.Fatalf("expected 2 files imported, got %d", cnt)
	}
}
