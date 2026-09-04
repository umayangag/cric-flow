package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
)

// PlayerStatusStore is the retirement ledger's persistence (migration 0009). It
// implements availability.Store; the rules themselves live in that package, so this file
// holds SQL and nothing else.
type PlayerStatusStore struct{}

// NewPlayerStatusStore returns the ledger store backed by the process's connection pool.
func NewPlayerStatusStore() *PlayerStatusStore { return &PlayerStatusStore{} }

// Event kinds written to player_status_event. They are read by people, not matched on by
// code, which is why they are prose-shaped rather than codes.
const (
	statusEventFlagged   = "flagged"
	statusEventPromoted  = "promoted"
	statusEventDemoted   = "demoted"
	statusEventUnflagged = "unflagged"
)

// ListFlags returns one actor's current retirement claims, keyed by player id.
func (s *PlayerStatusStore) ListFlags(ctx context.Context, actor string) (map[int64]availability.Flag, error) {
	if Pool == nil {
		return nil, errors.New("db pool not initialized")
	}
	rows, err := Pool.Query(ctx, `
		SELECT player_id, flagged_at, promoted_at, promoted_criterion, promoted_detail
		FROM player_status WHERE flagged_by = $1
	`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	flags := make(map[int64]availability.Flag)
	for rows.Next() {
		flag := availability.Flag{Actor: actor}
		var promotedAt *time.Time
		var criterion, detail *string
		if err := rows.Scan(&flag.PlayerID, &flag.FlaggedAt, &promotedAt, &criterion, &detail); err != nil {
			return nil, err
		}
		if promotedAt != nil {
			flag.PromotedAt = *promotedAt
		}
		if criterion != nil {
			flag.Criterion = *criterion
		}
		if detail != nil {
			flag.Detail = *detail
		}
		flags[flag.PlayerID] = flag
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return flags, nil
}

// RetirementEvidence reads what the corroboration criteria are allowed to see about a
// player: his most recent appearance in any format, and — once X-1a lands — his date of
// birth and career end date. Those two are not in this schema yet, so they come back
// zero and the criteria that read them report themselves unavailable.
func (s *PlayerStatusStore) RetirementEvidence(ctx context.Context, playerID int64) (availability.Evidence, error) {
	if Pool == nil {
		return availability.Evidence{}, errors.New("db pool not initialized")
	}
	evidence := availability.Evidence{PlayerID: playerID}
	var lastPlayed *time.Time
	err := Pool.QueryRow(ctx, `
		SELECT GREATEST(
		  (SELECT MAX(m.match_date) FROM batting_data bd JOIN match m ON m.match_id = bd.match_id
		    WHERE bd.player_id = $1),
		  (SELECT MAX(m.match_date) FROM bowling_data bw JOIN match m ON m.match_id = bw.match_id
		    WHERE bw.player_id = $1)
		)
	`, playerID).Scan(&lastPlayed)
	if err != nil {
		return availability.Evidence{}, err
	}
	if lastPlayed != nil {
		evidence.LastPlayed = *lastPlayed
	}
	return evidence, nil
}

// SaveFlag records a claim and, where a criterion corroborated it, raises the stored
// `player.is_retired` fact and records the promotion.
//
// One transaction, because the three writes are one decision: a promotion that reached
// `player` but not the event log would be a fact nobody could explain, which is the state
// D-12 found the column in.
func (s *PlayerStatusStore) SaveFlag(ctx context.Context, flag availability.Flag) error {
	if Pool == nil {
		return errors.New("db pool not initialized")
	}
	return withTx(ctx, func(tx pgx.Tx) error {
		var promotedAt any
		var criterion, detail any
		if flag.Promoted() {
			promotedAt, criterion, detail = flag.PromotedAt, flag.Criterion, flag.Detail
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO player_status
			  (player_id, flagged_by, flagged_at, promoted_at, promoted_criterion, promoted_detail)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (player_id, flagged_by) DO UPDATE SET
			  promoted_at = EXCLUDED.promoted_at,
			  promoted_criterion = EXCLUDED.promoted_criterion,
			  promoted_detail = EXCLUDED.promoted_detail
		`, flag.PlayerID, flag.Actor, flag.FlaggedAt, promotedAt, criterion, detail); err != nil {
			return err
		}
		if err := appendStatusEvent(ctx, tx, flag.PlayerID, flag.Actor, statusEventFlagged, nil, nil); err != nil {
			return err
		}
		if !flag.Promoted() {
			return nil
		}
		if _, err := tx.Exec(ctx,
			`UPDATE player SET is_retired = 1 WHERE id = $1`, flag.PlayerID); err != nil {
			return err
		}
		return appendStatusEvent(ctx, tx, flag.PlayerID, flag.Actor, statusEventPromoted,
			&flag.Criterion, &flag.Detail)
	})
}

// DeleteFlag removes one actor's claim. Where that claim had raised the stored fact, the
// fact is lowered again and the demotion recorded. It reports whether a claim was there.
//
// The fact is lowered only when no other actor's promoted claim still holds it up: a
// promotion is a fact about the player, and one user withdrawing his opinion does not
// unmake the evidence another user's promotion recorded.
func (s *PlayerStatusStore) DeleteFlag(
	ctx context.Context,
	actor string,
	playerID int64,
) (availability.Flag, bool, error) {
	if Pool == nil {
		return availability.Flag{}, false, errors.New("db pool not initialized")
	}
	var removed availability.Flag
	var existed bool
	err := withTx(ctx, func(tx pgx.Tx) error {
		removed = availability.Flag{PlayerID: playerID, Actor: actor}
		var promotedAt *time.Time
		var criterion, detail *string
		err := tx.QueryRow(ctx, `
			DELETE FROM player_status WHERE player_id = $1 AND flagged_by = $2
			RETURNING flagged_at, promoted_at, promoted_criterion, promoted_detail
		`, playerID, actor).Scan(&removed.FlaggedAt, &promotedAt, &criterion, &detail)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		existed = true
		if promotedAt != nil {
			removed.PromotedAt = *promotedAt
		}
		if criterion != nil {
			removed.Criterion = *criterion
		}
		if detail != nil {
			removed.Detail = *detail
		}
		if err := appendStatusEvent(ctx, tx, playerID, actor, statusEventUnflagged, nil, nil); err != nil {
			return err
		}
		if !removed.Promoted() {
			return nil
		}
		var stillPromoted bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM player_status WHERE player_id = $1 AND promoted_at IS NOT NULL)
		`, playerID).Scan(&stillPromoted); err != nil {
			return err
		}
		if stillPromoted {
			return nil
		}
		if _, err := tx.Exec(ctx,
			`UPDATE player SET is_retired = 0 WHERE id = $1`, playerID); err != nil {
			return err
		}
		reason := "the flag that corroborated it was withdrawn"
		return appendStatusEvent(ctx, tx, playerID, actor, statusEventDemoted, &removed.Criterion, &reason)
	})
	if err != nil {
		return availability.Flag{}, false, err
	}
	return removed, existed, nil
}

// appendStatusEvent writes one line of the ledger's history.
func appendStatusEvent(
	ctx context.Context,
	tx pgx.Tx,
	playerID int64,
	actor, event string,
	criterion, detail *string,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO player_status_event (player_id, actor, event, criterion, detail)
		VALUES ($1, $2, $3, $4, $5)
	`, playerID, actor, event, criterion, detail)
	return err
}

// withTx runs fn in a transaction, rolling back on any error.
func withTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := Pool.Begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(err, rollbackErr)
		}
		return err
	}
	return tx.Commit(ctx)
}
