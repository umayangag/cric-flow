package cricsheet

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// stubEntities records what identity resolution asked the entity cache for, which is the
// only observable difference between keying by name and keying by registry identifier.
type stubEntities struct {
	playerCalls      []stubPlayerCall
	oppositionCalls  []stubOppositionCall
	nextID           int64
	idsByKey         map[string]int64
	failPlayer       string
	failOpposition   string
	errPlayerFailure error
}

type stubPlayerCall struct {
	externalID string
	name       string
	nameAsOf   string
}

type stubOppositionCall struct {
	name   string
	gender string
}

var errStubEntities = errors.New("entity cache unavailable")

func newStubEntities() *stubEntities {
	return &stubEntities{idsByKey: map[string]int64{}, errPlayerFailure: errStubEntities}
}

func (s *stubEntities) GetPlayerID(_ context.Context, externalID, name, nameAsOf string) (int64, error) {
	s.playerCalls = append(s.playerCalls, stubPlayerCall{externalID: externalID, name: name, nameAsOf: nameAsOf})
	if name == s.failPlayer {
		return 0, s.errPlayerFailure
	}
	key := externalID
	if key == "" {
		key = "name:" + name
	}
	if id, ok := s.idsByKey[key]; ok {
		return id, nil
	}
	s.nextID++
	s.idsByKey[key] = s.nextID
	return s.nextID, nil
}

func (s *stubEntities) GetOppositionID(_ context.Context, name, gender string) (int64, error) {
	s.oppositionCalls = append(s.oppositionCalls, stubOppositionCall{name: name, gender: gender})
	if name == s.failOpposition {
		return 0, errStubEntities
	}
	key := name + "/" + gender
	if id, ok := s.idsByKey[key]; ok {
		return id, nil
	}
	s.nextID++
	s.idsByKey[key] = s.nextID
	return s.nextID, nil
}

func newTestIdentity(entities entityResolver, people map[string]string, gender, dateISO string) *matchIdentity {
	return &matchIdentity{
		entities: entities,
		registry: Registry{People: people}.PersonIDsByName(),
		gender:   gender,
		dateISO:  dateISO,
		path:     "match.json",
	}
}

func TestMatchIdentityPlayerID_UsesTheRegistryIdentifier(t *testing.T) {
	t.Parallel()

	entities := newStubEntities()
	identity := newTestIdentity(entities, map[string]string{"SR Taylor": "92cf79a8"}, "female", "2024-05-01")

	id, err := identity.PlayerID(context.Background(), "SR Taylor")

	require.NoError(t, err)
	require.Equal(t, int64(1), id)
	require.Len(t, entities.playerCalls, 1)
	assert.Equal(
		t,
		stubPlayerCall{externalID: "92cf79a8", name: "SR Taylor", nameAsOf: "2024-05-01"},
		entities.playerCalls[0],
	)
}

func TestMatchIdentityPlayerID_TwoPeopleUnderOneNameAreTwoPlayers(t *testing.T) {
	t.Parallel()

	// The defect this work removes: "SR Taylor" is two cricketers with 293 and 148 squad
	// appearances, and name-keying blended their careers into one row.
	entities := newStubEntities()
	ctx := context.Background()
	westIndies := newTestIdentity(entities, map[string]string{"SR Taylor": "92cf79a8"}, "female", "2024-05-01")
	england := newTestIdentity(entities, map[string]string{"SR Taylor": "a9231c3f"}, "male", "2024-06-01")

	first, err := westIndies.PlayerID(ctx, "SR Taylor")
	require.NoError(t, err)
	second, err := england.PlayerID(ctx, "SR Taylor")
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
}

func TestMatchIdentityPlayerID_OnePersonUnderTwoSpellingsIsOnePlayer(t *testing.T) {
	t.Parallel()

	// The other half: registry id f3a18a0c is spelled "NR Sciver" in 283 squads and
	// "NR Sciver-Brunt" in 171. Name-keying split one career across two rows.
	entities := newStubEntities()
	ctx := context.Background()
	before := newTestIdentity(entities, map[string]string{"NR Sciver": "f3a18a0c"}, "female", "2022-01-01")
	after := newTestIdentity(entities, map[string]string{"NR Sciver-Brunt": "f3a18a0c"}, "female", "2024-01-01")

	first, err := before.PlayerID(ctx, "NR Sciver")
	require.NoError(t, err)
	second, err := after.PlayerID(ctx, "NR Sciver-Brunt")
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

func TestMatchIdentityPlayerID_FallsBackToNameWhenTheSourceHasNoRegistryEntry(t *testing.T) {
	t.Parallel()

	entities := newStubEntities()
	identity := newTestIdentity(entities, map[string]string{"Someone Else": "0dc00542"}, "male", "2024-05-01")

	id, err := identity.PlayerID(context.Background(), "Unregistered Player")

	require.NoError(t, err)
	require.NotZero(t, id)
	require.Len(t, entities.playerCalls, 1)
	assert.Empty(t, entities.playerCalls[0].externalID, "an empty identifier is what selects the name-keyed fallback")
	assert.Equal(t, "Unregistered Player", entities.playerCalls[0].name)
}

func TestMatchIdentityPlayerID_TreatsAnEmptyRegistryValueAsAbsent(t *testing.T) {
	t.Parallel()

	entities := newStubEntities()
	identity := newTestIdentity(entities, map[string]string{"A Player": "   "}, "male", "2024-05-01")

	_, err := identity.PlayerID(context.Background(), "A Player")

	require.NoError(t, err)
	require.Len(t, entities.playerCalls, 1)
	assert.Empty(t, entities.playerCalls[0].externalID)
}

func TestMatchIdentityPlayerID_PropagatesResolutionFailure(t *testing.T) {
	t.Parallel()

	entities := newStubEntities()
	entities.failPlayer = "A Player"
	identity := newTestIdentity(entities, map[string]string{"A Player": "0dc00542"}, "male", "2024-05-01")

	_, err := identity.PlayerID(context.Background(), "A Player")

	require.ErrorIs(t, err, errStubEntities)
}

func TestMatchIdentityOppositionID_CarriesTheMatchGender(t *testing.T) {
	t.Parallel()

	// 130 of 394 team names are used by both a men's and a women's side.
	entities := newStubEntities()
	ctx := context.Background()
	womens := newTestIdentity(entities, nil, "female", "2024-05-01")
	mens := newTestIdentity(entities, nil, "male", "2024-05-01")

	womensID, err := womens.OppositionID(ctx, "Australia")
	require.NoError(t, err)
	mensID, err := mens.OppositionID(ctx, "Australia")
	require.NoError(t, err)

	assert.NotEqual(t, womensID, mensID)
	assert.Equal(t, []stubOppositionCall{
		{name: "Australia", gender: "female"},
		{name: "Australia", gender: "male"},
	}, entities.oppositionCalls)
}

func TestMatchIdentityOppositionID_PropagatesResolutionFailure(t *testing.T) {
	t.Parallel()

	entities := newStubEntities()
	entities.failOpposition = "Australia"
	identity := newTestIdentity(entities, nil, "male", "2024-05-01")

	_, err := identity.OppositionID(context.Background(), "Australia")

	require.ErrorIs(t, err, errStubEntities)
}

func TestMatchIdentity_SameNameOnBothSidesStillCollapsesToOnePlayer(t *testing.T) {
	t.Parallel()

	// Documented and deliberate: info.registry is keyed by name *within a file*, so two
	// namesakes in one match share one identifier and the source genuinely cannot say
	// which side each delivery belongs to. File 1130677 has 22 squad entries and 21
	// identifiers. SquadFromInfo drops the name from both squads for exactly this
	// reason; registry keying does not change it.
	entities := newStubEntities()
	identity := newTestIdentity(entities, map[string]string{"KV Sharma": "d1c1a9de"}, "male", "2018-04-01")
	ctx := context.Background()

	vidarbha, err := identity.PlayerID(ctx, "KV Sharma")
	require.NoError(t, err)
	railways, err := identity.PlayerID(ctx, "KV Sharma")
	require.NoError(t, err)

	assert.Equal(t, vidarbha, railways)

	_, ambiguous, err := SquadFromInfo(Info{
		Teams:   []string{"Vidarbha", "Railways"},
		Players: map[string][]string{"Vidarbha": {"KV Sharma", "A Player"}, "Railways": {"KV Sharma", "B Player"}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"KV Sharma"}, ambiguous, "dropped from both squads, as before")
}

func TestDisplayNames_LatestMatchDateWins(t *testing.T) {
	t.Parallel()

	names := newDisplayNames()
	names.observe(7, "NR Sciver", "2022-06-01")
	names.observe(7, "NR Sciver-Brunt", "2024-07-01")
	names.observe(7, "NR Sciver", "2023-01-01")

	assert.Equal(
		t,
		[]db.PlayerDisplayName{{PlayerID: 7, Name: "NR Sciver-Brunt", NameAsOf: "2024-07-01"}},
		names.rows(),
	)
}

func TestDisplayNames_ResultDoesNotDependOnObservationOrder(t *testing.T) {
	t.Parallel()

	// The import is concurrent, so a rule that depended on arrival order would make two
	// runs of the same dataset disagree on a player's name.
	forwards := newDisplayNames()
	forwards.observe(1, "A Name", "2020-01-01")
	forwards.observe(1, "Z Name", "2020-01-01")
	backwards := newDisplayNames()
	backwards.observe(1, "Z Name", "2020-01-01")
	backwards.observe(1, "A Name", "2020-01-01")

	assert.Equal(t, forwards.rows(), backwards.rows())
}

func TestDisplayNames_IgnoresUnusableObservations(t *testing.T) {
	t.Parallel()

	names := newDisplayNames()
	names.observe(0, "No Player Id", "2024-01-01")
	names.observe(3, "   ", "2024-01-01")

	assert.Empty(t, names.rows())
}

func TestDisplayNames_RowsAreOrderedByPlayerID(t *testing.T) {
	t.Parallel()

	names := newDisplayNames()
	names.observe(9, "Nine", "2024-01-01")
	names.observe(2, "Two", "2024-01-01")
	names.observe(5, "Five", "2024-01-01")

	rows := names.rows()
	require.Len(t, rows, 3)
	assert.Equal(t, []int64{2, 5, 9}, []int64{rows[0].PlayerID, rows[1].PlayerID, rows[2].PlayerID})
}

func TestParse_ReadsTheRegistryAndItsIdentifiers(t *testing.T) {
	t.Parallel()

	raw := `{"info":{"dates":["2024-05-01"],"match_type":"T20","team_type":"club","gender":"female",
		"teams":["Alpha","Beta"],
		"registry":{"people":{"NR Sciver-Brunt":"f3a18a0c","KH Sciver-Brunt":"6a434bd3"}}},"innings":[]}`

	m, err := Parse(strings.NewReader(raw))

	require.NoError(t, err)
	people := m.Info.Registry.PersonIDsByName()
	assert.Equal(t, "f3a18a0c", people["NR Sciver-Brunt"])
	assert.NotContains(t, people, "Not In This File",
		"a miss is a fact about the source, reported so the caller can log it")
}

func TestRegistryPersonIDsByName_TrimsBothSidesOfTheMapping(t *testing.T) {
	t.Parallel()

	// Four registry keys in the dataset carry a trailing space while the squad entry
	// naming the same person does not; an exact-match lookup dropped them onto the
	// name-keyed fallback and split one career across two rows.
	registry := Registry{People: map[string]string{"Lalchhuanliana ": " 2ca0f319 ", "  ": "abc", "X": ""}}

	people := registry.PersonIDsByName()

	assert.Equal(t, map[string]string{"Lalchhuanliana": "2ca0f319"}, people)
}

// TestRegistryPersonIDsByName_DuplicateAfterTrim_ResolvesTheSameWayEveryTime pins
// IMPORT-15's other half. No file in the current archive has two raw registry keys that
// trim to the same name (see the doc comment on PersonIDsByName), but ranging over
// r.People in whatever order Go's runtime picks -- which varies from call to call, not
// just from process to process -- made "the later key wins" nondeterministic if one ever
// did. Sorting the raw keys first makes the outcome a property of the keys, not of when
// the map happened to be walked.
func TestRegistryPersonIDsByName_DuplicateAfterTrim_ResolvesTheSameWayEveryTime(t *testing.T) {
	t.Parallel()

	// Built with assignments, not a map literal: a literal's trailing-space key reads as
	// a typo to the linter, though it is the exact shape a real registry entry takes
	// (see TestRegistryPersonIDsByName_TrimsBothSidesOfTheMapping).
	people := map[string]string{}
	people["Lalchhuanliana "] = "2ca0f319"
	people["Lalchhuanliana"] = "zzz00000"
	registry := Registry{People: people}

	var want string
	for i := range 200 {
		people := registry.PersonIDsByName()
		got := people["Lalchhuanliana"]
		if i == 0 {
			want = got
		}
		require.Equal(t, want, got, "call %d picked a different identifier than call 0", i)
	}
	// Sorted, "Lalchhuanliana" precedes "Lalchhuanliana " (a string precedes itself plus
	// a trailing byte), so the space-trailing key is assigned last and wins.
	require.Equal(t, "2ca0f319", want)
}

func TestMatchIdentityPlayerID_ResolvesANameTheRegistrySpellsWithTrailingSpace(t *testing.T) {
	t.Parallel()

	entities := newStubEntities()
	identity := newTestIdentity(entities, map[string]string{"Lalchhuanliana ": "2ca0f319"}, "male", "2021-01-11")

	_, err := identity.PlayerID(context.Background(), "Lalchhuanliana")

	require.NoError(t, err)
	require.Len(t, entities.playerCalls, 1)
	assert.Equal(t, "2ca0f319", entities.playerCalls[0].externalID)
}
