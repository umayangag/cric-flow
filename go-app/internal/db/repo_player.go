package db

import (
	"context"
	"errors"
)

// Player represents the player table row
type Player struct {
	ID             int64
	Name           string
	IsWicketKeeper int16
	IsRetired      int16
}

// PlayerDisplayName is one player's chosen display name and the match date it was read
// from. See UpdatePlayerDisplayNames.
type PlayerDisplayName struct {
	PlayerID int64
	Name     string
	NameAsOf string // YYYY-MM-DD
}

// GetPlayerByID returns a player by their ID.
func GetPlayerByID(ctx context.Context, id int64) (*Player, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	row := QueryRow(
		ctx,
		`SELECT id, player_name, is_wicket_keeper, is_retired FROM player WHERE id = $1`,
		id,
	)
	p := &Player{}
	if err := row.Scan(&p.ID, &p.Name, &p.IsWicketKeeper, &p.IsRetired); err != nil {
		return nil, err
	}
	return p, nil
}

// GetOrCreatePlayer fetches a player id by Cricsheet person identifier, creating the row
// if it is new, and returns the display name currently stored against it.
//
// The identifier is the identity; the name is an attribute of it. Passing an empty
// externalID takes the fallback path, where the name is the identity again -- the old
// behaviour, kept for a source that has no registry entry, and deliberately reachable
// only through an explicit empty value so the caller has to decide to use it.
//
// nameAsOf is the match date the name was read from. It is returned alongside so the
// caller can tell whether a later match has since supplied a newer spelling; the row
// itself is not renamed here, because under a concurrent import the last writer is
// whichever goroutine happened to finish last. UpdatePlayerDisplayNames settles it once,
// after the whole import, from a rule that does not depend on order.
func GetOrCreatePlayer(
	ctx context.Context,
	externalID, name, nameAsOf string,
) (id int64, storedName string, err error) {
	if Pool == nil {
		return 0, "", errors.New("db pool not initialized")
	}
	if externalID == "" {
		err = Pool.QueryRow(ctx, `INSERT INTO player(player_name, name_as_of) VALUES($1, $2::date)
			ON CONFLICT (player_name) WHERE external_id IS NULL
			DO UPDATE SET player_name = EXCLUDED.player_name
			RETURNING id, player_name`, name, nullableDate(nameAsOf)).Scan(&id, &storedName)
		return id, storedName, err
	}
	err = Pool.QueryRow(ctx, `INSERT INTO player(external_id, player_name, name_as_of) VALUES($1, $2, $3::date)
		ON CONFLICT (external_id) DO UPDATE SET external_id = EXCLUDED.external_id
		RETURNING id, player_name`, externalID, name, nullableDate(nameAsOf)).Scan(&id, &storedName)
	return id, storedName, err
}

// UpdatePlayerDisplayNames renames players to the spelling their most recent match used.
//
// Called once at the end of an import with the winning name per player. The update is
// guarded on name_as_of so an incremental import of an older file cannot revert a name,
// and so re-running an unchanged import writes nothing.
//
// Rows on the name-keyed fallback are left alone: for those the name *is* the identity,
// so renaming one would change who it refers to.
func UpdatePlayerDisplayNames(ctx context.Context, names []PlayerDisplayName) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	if len(names) == 0 {
		return nil
	}
	ids := make([]int64, len(names))
	values := make([]string, len(names))
	dates := make([]string, len(names))
	for i, n := range names {
		ids[i], values[i], dates[i] = n.PlayerID, n.Name, n.NameAsOf
	}
	_, err := Pool.Exec(ctx, `UPDATE player p
		SET player_name = u.name, name_as_of = u.name_as_of
		FROM (SELECT unnest($1::bigint[]) AS id,
		             unnest($2::text[]) AS name,
		             unnest($3::date[]) AS name_as_of) u
		WHERE p.id = u.id
		  AND p.external_id IS NOT NULL
		  AND (p.name_as_of IS NULL OR u.name_as_of >= p.name_as_of)
		  AND (p.player_name, p.name_as_of) IS DISTINCT FROM (u.name, u.name_as_of)`,
		ids, values, dates)
	return err
}

// nullableDate turns an empty date string into a SQL NULL, so a caller with no date is
// recorded as having none rather than as 0001-01-01.
func nullableDate(iso string) any {
	if iso == "" {
		return nil
	}
	return iso
}
