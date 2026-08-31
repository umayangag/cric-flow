package cricsheet

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// entityResolver is the slice of the process entity cache that identity resolution
// needs. Narrow so a match's identities can be resolved in a test without a database.
type entityResolver interface {
	GetPlayerID(ctx context.Context, externalID, name, nameAsOf string) (int64, error)
	GetOppositionID(ctx context.Context, name, gender string) (int64, error)
}

// matchIdentity turns the names in one match file into database identities.
//
// It exists because identity is a property of the *file*, not of a name: the Cricsheet
// person identifier is only knowable through that file's info.registry, and a team's
// gender only through its info.gender. Resolving through a per-match value means the
// ingest code below can go on passing names around, while every name that reaches the
// database has been attached to the identity its match gives it.
type matchIdentity struct {
	entities entityResolver
	// The file's identifiers, keyed by trimmed name; see Registry.PersonIDsByName.
	registry map[string]string
	gender   string
	dateISO  string
	path     string

	// Where the display name each player should end up with is collected. Nil for a
	// single-file import, which has nothing to settle across files.
	names *displayNames
}

// PlayerID resolves a player name in this match to a player id.
//
// When the file's registry has no entry for the name the lookup falls back to keying by
// name -- the pre-identity behaviour, and the only remaining path that can merge two
// people. It is logged every time, because a silent fallback would reintroduce the bug it
// exists to survive. Coverage across the current 22,734 files is total, so a warning here
// means the dataset has changed shape.
func (m *matchIdentity) PlayerID(ctx context.Context, name string) (int64, error) {
	externalID, ok := m.registry[strings.TrimSpace(name)]
	if !ok {
		slog.Warn("cricsheet: no registry entry for player, falling back to name identity",
			slog.String("file", m.path),
			slog.String("match_date", m.dateISO),
			slog.String("player", name))
	}
	id, err := m.entities.GetPlayerID(ctx, externalID, name, m.dateISO)
	if err != nil {
		return 0, err
	}
	if m.names != nil {
		m.names.observe(id, name, m.dateISO)
	}
	return id, nil
}

// OppositionID resolves a team name in this match to an opposition id, under the match's
// gender. 130 of the 394 names in the dataset belong to both a men's and a women's side.
func (m *matchIdentity) OppositionID(ctx context.Context, name string) (int64, error) {
	return m.entities.GetOppositionID(ctx, name, m.gender)
}

// displayNames collects, across a whole import, which spelling of a name each player
// should be shown under.
//
// A person's name changes -- registry id f3a18a0c appears as "NR Sciver" in 283 squads
// and "NR Sciver-Brunt" in 171 -- and the one to display is the current one. The most
// frequent spelling picks the former name and changes its mind as matches accumulate;
// the first spelling written picks whichever goroutine won a race, which would make two
// runs of the same dataset disagree. The rule here is the name from the latest match
// date, tie-broken by the greater string, so the result does not depend on the order
// files happen to be processed in.
type displayNames struct {
	mu   sync.Mutex
	best map[int64]db.PlayerDisplayName
}

func newDisplayNames() *displayNames {
	return &displayNames{best: map[int64]db.PlayerDisplayName{}}
}

func (d *displayNames) observe(playerID int64, name, dateISO string) {
	name = strings.TrimSpace(name)
	if playerID == 0 || name == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	current, seen := d.best[playerID]
	if seen && !supersedes(dateISO, name, current.NameAsOf, current.Name) {
		return
	}
	d.best[playerID] = db.PlayerDisplayName{PlayerID: playerID, Name: name, NameAsOf: dateISO}
}

// supersedes reports whether (date, name) should replace (currentDate, currentName).
// Dates are ISO-8601, which compares correctly as a string.
func supersedes(date, name, currentDate, currentName string) bool {
	if date != currentDate {
		return date > currentDate
	}
	return name > currentName
}

// rows returns the chosen names in player-id order, so the update statement and any test
// over it are deterministic.
func (d *displayNames) rows() []db.PlayerDisplayName {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]db.PlayerDisplayName, 0, len(d.best))
	for _, row := range d.best {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PlayerID < out[j].PlayerID })
	return out
}
