package db

import (
	"context"
	"strconv"
	"strings"
)

// KeeperRepo exposes minimal operations needed by the import-keepers command.
// It is implemented in terms of the package-level DB helpers so it is testable
// via SetDB with pgxmock-compatible adapters.
type KeeperRepo interface {
	// CountPlayersByLowerName returns the number of players matching the exact lower(name).
	CountPlayersByLowerName(ctx context.Context, lowerName string) (int64, error)
	// SetIsWicketKeeperByLowerName sets the is_wicket_keeper flag for exact lower(name).
	SetIsWicketKeeperByLowerName(ctx context.Context, value int, lowerName string) (int64, error)
	// ZeroKeepersExcept sets is_wicket_keeper=0 for players whose lower(name) is NOT in provided list.
	ZeroKeepersExcept(ctx context.Context, lowerNames []string) (int64, error)
}

// CountPlayersByLowerName returns count of players whose lower(player_name) equals lowerName.
func CountPlayersByLowerName(ctx context.Context, lowerName string) (int64, error) {
	var n int64
	if err := QueryRow(ctx, `SELECT COUNT(1) FROM player WHERE lower(player_name) = $1`, lowerName).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// SetIsWicketKeeperByLowerName updates the flag for rows matching the provided lower-cased name.
func SetIsWicketKeeperByLowerName(ctx context.Context, value int, lowerName string) (int64, error) {
	rows, err := Query(ctx, `UPDATE player SET is_wicket_keeper = $1 WHERE lower(player_name) = $2 RETURNING 1`, value, lowerName)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var affected int64
	for rows.Next() {
		var one int
		if err := rows.Scan(&one); err != nil {
			return 0, err
		}
		affected++
	}
	return affected, nil
}

// ZeroKeepersExcept sets is_wicket_keeper=0 where lower(player_name) NOT IN list.
func ZeroKeepersExcept(ctx context.Context, lowerNames []string) (int64, error) {
	if len(lowerNames) == 0 {
		if err := Exec(ctx, `UPDATE player SET is_wicket_keeper = 0`); err != nil {
			return 0, err
		}
		var n int64
		if err := QueryRow(ctx, `SELECT COUNT(1) FROM player`).Scan(&n); err != nil {
			return 0, err
		}
		return n, nil
	}
	// Build numbered placeholders and args.
	args := make([]any, 0, len(lowerNames))
	for _, s := range lowerNames {
		args = append(args, s)
	}
	b := strings.Builder{}
	b.WriteString(`UPDATE player SET is_wicket_keeper = 0 WHERE lower(player_name) NOT IN (`)
	for i := range lowerNames {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("$")
		b.WriteString(strconv.Itoa(i + 1))
	}
	b.WriteString(`) RETURNING 1`)
	rows, err := Query(ctx, b.String(), args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var affected int64
	for rows.Next() {
		var one int
		if err := rows.Scan(&one); err != nil {
			return 0, err
		}
		affected++
	}
	return affected, nil
}
