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

	// The spellings this file used, held back until its transaction commits. Settlement
	// is a computation over what the archive holds, and a file whose rows rolled back put
	// nothing in the archive; letting it vote anyway was how a failed file could still
	// rename a player (IMPORT-13). Not guarded by a mutex: one matchIdentity belongs to
	// one file, and one file is read by one goroutine.
	observed []db.PlayerDisplayName
}

// SettleObservedNames hands this file's spellings to the import-wide collector. Call it
// once the file's transaction has committed and not before; a rolled-back file has no
// spelling to contribute.
func (m *matchIdentity) SettleObservedNames() {
	if m.names == nil {
		return
	}
	for i := range m.observed {
		m.names.observe(m.observed[i].PlayerID, m.observed[i].Name, m.observed[i].NameAsOf)
	}
	m.observed = nil
}

// PlayerID resolves a player name in this match to a player id.
//
// When the file's registry has no entry for the name the lookup falls back to keying by
// name -- the pre-identity behaviour. It is logged every time, because a silent fallback
// would reintroduce the bug it exists to survive. Coverage across the current 22,905 files
// is total (checked directly, not merely carried forward from the 22,734-file count this
// once read), so a warning here means the dataset has changed shape.
//
// The fallback itself can no longer duplicate a person who is already known under a real
// identifier: GetOrCreatePlayer looks for an existing (name, non-null external_id) row
// before it mints a name-keyed one (IMPORT-15). It can still merge two different people who
// happen to share an exact display name and both lack a registry entry -- the same limit
// the pre-identity importer had -- which is why the warning stays loud rather than going
// quiet now that the common case is safe.
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
		m.observed = append(m.observed, db.PlayerDisplayName{PlayerID: id, Name: name, NameAsOf: m.dateISO})
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
