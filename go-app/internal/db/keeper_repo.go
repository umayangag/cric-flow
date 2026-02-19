package db

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/umayangag/cric-flow/go-app/internal/db/scanx"
)

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
	rows, err := Query(
		ctx,
		`UPDATE player SET is_wicket_keeper = $1 WHERE lower(player_name) = $2 RETURNING 1`,
		value,
		lowerName,
	)
	if err != nil {
		return 0, err
	}
	return scanx.CountReturningOnes(rows)
}

// BatchSetIsWicketKeeper updates multiple players' wicket-keeper status in two batches (one for value 0, one for value 1).
func BatchSetIsWicketKeeper(ctx context.Context, targets map[string]int) (int64, error) {
	var total int64
	groups := map[int][]string{}
	for name, v := range targets {
		groups[v] = append(groups[v], strings.ToLower(name))
	}

	for v, names := range groups {
		if len(names) == 0 {
			continue
		}
		rows, err := Query(
			ctx,
			`UPDATE player SET is_wicket_keeper = $1 WHERE lower(player_name) = ANY($2) RETURNING 1`,
			v,
			names,
		)
		if err != nil {
			return total, err
		}
		count, err := scanx.CountReturningOnes(rows)
		if err != nil {
			return total, err
		}
		total += count
	}
	return total, nil
}

// ZeroKeepersExcept sets is_wicket_keeper=0 where lower(player_name) NOT IN list.
func ZeroKeepersExcept(ctx context.Context, lowerNames []string) (int64, error) {
	if len(lowerNames) == 0 {
		slog.Warn("ZeroKeepersExcept called with empty list")
		return 0, errors.New("empty list")
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

	// Build numbered placeholders and args.
	args := make([]any, 0, len(lowerNames))
	for _, s := range lowerNames {
		args = append(args, s)
	}
	rows, err := Query(ctx, b.String(), args...)
	if err != nil {
		return 0, err
	}
	return scanx.CountReturningOnes(rows)
}
