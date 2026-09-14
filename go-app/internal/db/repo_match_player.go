package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// MatchPlayer is one player a side picked for one match: a row of match_player.
//
// IsReplacement marks the member who joined after the match started -- a concussion
// substitute, an impact player, a supersub -- so a reader that wants the eleven that
// started can leave him out while the row still records that he played (migration 0020).
type MatchPlayer struct {
	MatchID       int64
	PlayerID      int64
	OppositionID  int64
	IsReplacement bool
}

// matchPlayerColumns is the number of values one match_player row is inserted with.
const matchPlayerColumns = 4

// ReplaceMatchPlayersTx makes match_player hold exactly rows for the given match.
//
// Replace rather than upsert: a re-import of a corrected Cricsheet file can drop a
// player from a side, and an upsert would leave that player behind as a member of a
// team that did not pick them. Since the whole point of this table is that its
// membership is not a function of the result, a membership that silently accumulated
// across imports would be worse than not having it.
//
// Both statements run on the caller's transaction, so a failed import leaves the
// previous squad intact rather than a deleted one.
func ReplaceMatchPlayersTx(ctx context.Context, tx CopyFromTx, matchID int64, rows []MatchPlayer) error {
	if err := tx.Exec(ctx, `DELETE FROM match_player WHERE match_id = $1`, matchID); err != nil {
		return fmt.Errorf("delete match_player for match %d: %w", matchID, err)
	}
	if len(rows) == 0 {
		return nil
	}
	// A squad is a couple of dozen rows at most, so one multi-row INSERT beats the
	// temp-table-and-COPY dance the ball-by-ball batches need.
	placeholders := make([]string, 0, len(rows))
	args := make([]any, 0, len(rows)*matchPlayerColumns)
	for i := range rows {
		base := i * matchPlayerColumns
		placeholders = append(
			placeholders,
			"($"+strconv.Itoa(base+1)+", $"+strconv.Itoa(base+2)+", $"+strconv.Itoa(base+3)+
				", $"+strconv.Itoa(base+4)+")",
		)
		args = append(args, rows[i].MatchID, rows[i].PlayerID, rows[i].OppositionID, rows[i].IsReplacement)
	}
	sql := `INSERT INTO match_player (match_id, player_id, opposition_id, is_replacement) VALUES ` +
		strings.Join(placeholders, ", ")
	if err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("insert %d match_player rows for match %d: %w", len(rows), matchID, err)
	}
	return nil
}
