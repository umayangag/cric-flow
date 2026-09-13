package db

import (
	"context"
	"fmt"
)

// matchFactDelete names one table the Cricsheet importer fills per match, together with
// the statement that clears that match out of it. The statement is written out rather
// than composed from the table name so that nothing here builds SQL by concatenation.
type matchFactDelete struct {
	table     string
	statement string
}

// matchFactDeletes is every table one match's import writes rows into, except the match
// row itself (upserted, because the match still exists) and match_player (replaced by
// ReplaceMatchPlayersTx, which owns both halves of its own replace).
//
// A table added to the importer belongs in this list. Nothing enforces that, which is why
// the list is here and not spread across the repositories that insert into each table.
var matchFactDeletes = []matchFactDelete{
	// ball_event_wicket references ball_event, so it goes first.
	{table: "ball_event_wicket", statement: `DELETE FROM ball_event_wicket WHERE match_id = $1`},
	{table: "ball_event", statement: `DELETE FROM ball_event WHERE match_id = $1`},
	{table: "fielding_event", statement: `DELETE FROM fielding_event WHERE match_id = $1`},
	{table: "batting_data", statement: `DELETE FROM batting_data WHERE match_id = $1`},
	{table: "bowling_data", statement: `DELETE FROM bowling_data WHERE match_id = $1`},
	{table: "fielding_data", statement: `DELETE FROM fielding_data WHERE match_id = $1`},
	{table: "match_inning", statement: `DELETE FROM match_inning WHERE match_id = $1`},
}

// DeleteMatchFactsTx clears every row a previous import wrote for one match, so that the
// import running now replaces the match instead of merging into what is already there.
//
// Replace rather than upsert, for the same reason as ReplaceMatchPlayersTx. Cricsheet
// republishes a corrected file under the same id, and a correction can *remove* things: an
// innings that was never played, a delivery that was scored twice, a player who was not in
// the side. An upsert refreshes the rows the new file still names and leaves every other
// row behind, so what the database describes becomes the union of every version of the file
// ever imported rather than the current one. ball_event had it worse still: its insert was
// ON CONFLICT DO NOTHING, so a re-import of an already-imported match changed nothing at
// all, and it has no foreign key to match, so nothing in the schema would ever have cleaned
// the rows up. That is also what stopped an importer fix from reaching an already-imported
// match without a manual truncate -- P-3 found 452 catches credited to the bowler, and a
// re-import could not have corrected a single one of them.
//
// Every statement runs on the caller's transaction, so a failed import leaves the previous
// rows intact rather than a half-deleted match.
func DeleteMatchFactsTx(ctx context.Context, tx CopyFromTx, matchID int64) error {
	for _, d := range matchFactDeletes {
		if err := tx.Exec(ctx, d.statement, matchID); err != nil {
			return fmt.Errorf("delete %s for match %d: %w", d.table, matchID, err)
		}
	}
	return nil
}
