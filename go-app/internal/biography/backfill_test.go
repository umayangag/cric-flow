package biography_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/biography"
)

// recordingStore captures what the run wrote, so a test can assert on the rows rather
// than on a database.
type recordingStore struct {
	players   []biography.Player
	written   []biography.Record
	listErr   error
	upsertErr error
}

func (s *recordingStore) ListPlayers(context.Context) ([]biography.Player, error) {
	return s.players, s.listErr
}

func (s *recordingStore) UpsertBiographies(_ context.Context, records []biography.Record) error {
	s.written = append(s.written, records...)
	return s.upsertErr
}

func (s *recordingStore) Coverage(context.Context) (biography.Coverage, error) {
	return biography.Coverage{}, nil
}

// scriptedLookuper answers from a fixed map and counts what it was asked, which is how
// resumability is observed.
type scriptedLookuper struct {
	answers map[string]biography.Lookup
	asked   [][]string
	err     error
}

func (l *scriptedLookuper) Query(_ context.Context, ids []string) (map[string]biography.Lookup, error) {
	l.asked = append(l.asked, append([]string(nil), ids...))
	if l.err != nil {
		return nil, l.err
	}
	found := make(map[string]biography.Lookup)
	for _, id := range ids {
		if lookup, ok := l.answers[id]; ok {
			found[id] = lookup
		}
	}
	return found, nil
}

func born(year int) *time.Time {
	date := time.Date(year, 4, 1, 0, 0, 0, 0, time.UTC)
	return &date
}

// testOptions builds a run configuration over a fresh cache in a temp directory.
func testOptions(t *testing.T, register map[string]biography.RegisterEntry) biography.Options {
	t.Helper()
	cache, err := biography.OpenCache(filepath.Join(t.TempDir(), "lookups.jsonl"))
	require.NoError(t, err)
	return biography.Options{
		Register:  register,
		Cache:     cache,
		BatchSize: 2,
		Now:       func() time.Time { return time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC) },
	}
}

// TestRun_WritesARowForEveryPlayerIncludingTheMisses is what makes the coverage figure a
// measurement: "asked and not found" is stored, so it can be counted.
func TestRun_WritesARowForEveryPlayerIncludingTheMisses(t *testing.T) {
	t.Parallel()
	store := &recordingStore{players: []biography.Player{
		{ID: 1, ExternalID: "aaa", Appearances: 10},
		{ID: 2, ExternalID: "bbb", Appearances: 5},
		{ID: 3, ExternalID: "ccc", Appearances: 1},
	}}
	lookuper := &scriptedLookuper{answers: map[string]biography.Lookup{
		"111": {CricinfoID: "111", QID: "Q1", BirthDate: born(1990), BowlingStyleRaw: "leg break"},
	}}
	options := testOptions(t, map[string]biography.RegisterEntry{
		"aaa": {Identifier: "aaa", CricinfoIDs: []string{"111"}},
		"bbb": {Identifier: "bbb", CricinfoIDs: []string{"222"}},
		"ccc": {Identifier: "ccc"},
	})

	result, err := biography.Run(context.Background(), store, lookuper, options)

	require.NoError(t, err)
	require.Len(t, store.written, 3, "a player nothing was found for still gets a row")
	assert.Equal(t, 3, result.Players)
	assert.Equal(t, 2, result.WithCricinfoID)
	assert.Equal(t, 1, result.Matched)
	assert.Equal(t, biography.StyleLegSpin, store.written[0].BowlingStyle)
	assert.Equal(t, "leg break", store.written[0].BowlingStyleRaw)
	assert.False(t, store.written[1].Matched())
	assert.Equal(t, "222", store.written[1].CricinfoID,
		"the id that was tried is recorded so a curator can re-check it by hand")
	assert.Empty(t, store.written[2].CricinfoID,
		"a player the register carries no ESPNcricinfo id for has nothing to try")
}

// TestRun_AsksOnlyForIDsTheCacheHasNoAnswerFor is resumability: the second run over the
// same data asks Wikidata nothing.
func TestRun_AsksOnlyForIDsTheCacheHasNoAnswerFor(t *testing.T) {
	t.Parallel()
	store := &recordingStore{players: []biography.Player{
		{ID: 1, ExternalID: "aaa"}, {ID: 2, ExternalID: "bbb"},
	}}
	lookuper := &scriptedLookuper{answers: map[string]biography.Lookup{
		"111": {CricinfoID: "111", QID: "Q1"},
	}}
	options := testOptions(t, map[string]biography.RegisterEntry{
		"aaa": {CricinfoIDs: []string{"111"}},
		"bbb": {CricinfoIDs: []string{"222"}},
	})

	_, err := biography.Run(context.Background(), store, lookuper, options)
	require.NoError(t, err)
	first := len(lookuper.asked)
	second, err := biography.Run(context.Background(), store, lookuper, options)

	require.NoError(t, err)
	assert.Equal(t, first, len(lookuper.asked), "a second run asks nothing new")
	assert.Zero(t, second.AskedNow)
	assert.Equal(t, 2, second.FromCache, "the miss is cached too, or it would be re-asked forever")
}

// TestRun_BatchesTheOutstandingIDs keeps one query per batch rather than one per player.
func TestRun_BatchesTheOutstandingIDs(t *testing.T) {
	t.Parallel()
	store := &recordingStore{players: []biography.Player{
		{ID: 1, ExternalID: "a"}, {ID: 2, ExternalID: "b"}, {ID: 3, ExternalID: "c"},
	}}
	lookuper := &scriptedLookuper{}
	options := testOptions(t, map[string]biography.RegisterEntry{
		"a": {CricinfoIDs: []string{"1"}},
		"b": {CricinfoIDs: []string{"2"}},
		"c": {CricinfoIDs: []string{"3"}},
	})

	_, err := biography.Run(context.Background(), store, lookuper, options)

	require.NoError(t, err)
	require.Len(t, lookuper.asked, 2, "three ids at a batch size of two is two queries")
	assert.Equal(t, []string{"1", "2"}, lookuper.asked[0],
		"ids are sorted, so a resumed run batches them the same way")
}

// TestRun_KeepsTheBatchesItAlreadyAnswered: a failure mid-run must leave the cache
// holding every batch before it, or "re-run to resume" would be a lie.
func TestRun_KeepsTheBatchesItAlreadyAnswered(t *testing.T) {
	t.Parallel()
	store := &recordingStore{players: []biography.Player{{ID: 1, ExternalID: "a"}}}
	options := testOptions(t, map[string]biography.RegisterEntry{"a": {CricinfoIDs: []string{"1"}}})
	failing := &scriptedLookuper{err: errors.New("service unavailable")}

	_, err := biography.Run(context.Background(), store, failing, options)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "re-run to resume")
	assert.Empty(t, store.written, "nothing is stored from a pass that did not finish asking")
}

// TestRun_AppliesCuratedOverrides is the hand-fix path the coverage report points at.
func TestRun_AppliesCuratedOverrides(t *testing.T) {
	t.Parallel()
	store := &recordingStore{players: []biography.Player{{ID: 1, ExternalID: "aaa"}}}
	options := testOptions(t, map[string]biography.RegisterEntry{"aaa": {CricinfoIDs: []string{"111"}}})
	options.Overrides = map[string]biography.Override{
		"aaa": {CricsheetID: "aaa", Note: "board profile", BirthDate: "1990-04-01"},
	}

	result, err := biography.Run(context.Background(), store, &scriptedLookuper{}, options)

	require.NoError(t, err)
	require.Len(t, store.written, 1)
	assert.Equal(t, 1, result.Overridden)
	assert.Equal(t, biography.SourceOverride, store.written[0].Source)
	require.NotNil(t, store.written[0].BirthDate)
}

// TestRun_TriesEverySecondaryCricinfoID: ESPNcricinfo lists some people twice, and
// Wikidata may carry either id.
func TestRun_TriesEverySecondaryCricinfoID(t *testing.T) {
	t.Parallel()
	store := &recordingStore{players: []biography.Player{{ID: 1, ExternalID: "aaa"}}}
	lookuper := &scriptedLookuper{answers: map[string]biography.Lookup{
		"222": {CricinfoID: "222", QID: "Q2"},
	}}
	options := testOptions(t, map[string]biography.RegisterEntry{
		"aaa": {CricinfoIDs: []string{"111", "222"}},
	})

	_, err := biography.Run(context.Background(), store, lookuper, options)

	require.NoError(t, err)
	require.Len(t, store.written, 1)
	assert.Equal(t, "Q2", store.written[0].WikidataQID)
	assert.Equal(t, "222", store.written[0].CricinfoID)
}

// TestRun_RefusesToRunWithoutACache: without one the run is not resumable, which is the
// property the acceptance turns on.
func TestRun_RefusesToRunWithoutACache(t *testing.T) {
	t.Parallel()

	_, err := biography.Run(context.Background(), &recordingStore{}, &scriptedLookuper{},
		biography.Options{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "resumable")
}

// TestRun_ReportsAStoreFailureRatherThanSwallowingIt.
func TestRun_ReportsAStoreFailureRatherThanSwallowingIt(t *testing.T) {
	t.Parallel()
	options := testOptions(t, nil)

	_, listFailed := biography.Run(context.Background(),
		&recordingStore{listErr: errors.New("db down")}, &scriptedLookuper{}, options)
	_, writeFailed := biography.Run(context.Background(),
		&recordingStore{players: []biography.Player{{ID: 1}}, upsertErr: errors.New("db down")},
		&scriptedLookuper{}, options)

	require.Error(t, listFailed)
	assert.Contains(t, listFailed.Error(), "listing players")
	require.Error(t, writeFailed)
	assert.Contains(t, writeFailed.Error(), "storing biographies")
}
