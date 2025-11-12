package importkeepers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// Runner orchestrates the import/preview flow using an abstract DB.
type Runner struct {
	DB DB
}

func NewRunner(db DB) Runner { return Runner{DB: db} }

// Preview logs the planned changes without mutating the database.
func (r Runner) Preview(ctx context.Context, targets map[string]int, othersZero bool) error {
	matched := 0
	for name, v := range targets {
		q := `SELECT COUNT(1) FROM player WHERE lower(player_name) = $1`
		cnt, err := r.DB.QueryRowCount(ctx, q, name)
		if err != nil {
			return err
		}
		if cnt == 0 {
			slog.Warn("no player matched for name", slog.String("name", name))
		} else {
			matched += int(cnt)
			slog.Info("PLAN: set is_wicket_keeper", slog.Int("value", v), slog.Int64("rows", cnt), slog.String("name", name))
		}
	}
	if othersZero {
		slog.Info("PLAN: set is_wicket_keeper=0 for players NOT in provided CSV")
	}
	slog.Info("dry-run summary", slog.Int("targets", len(targets)), slog.Int("matched", matched))
	return nil
}

// Apply performs the updates. When othersZero is true, sets is_wicket_keeper=0 for players not listed.
func (r Runner) Apply(ctx context.Context, targets map[string]int, othersZero bool) error {
	upd := `UPDATE player SET is_wicket_keeper = $1 WHERE lower(player_name) = $2`
	for name, v := range targets {
		if _, err := r.DB.Exec(ctx, upd, v, name); err != nil {
			return fmt.Errorf("update keeper for '%s': %w", name, err)
		}
	}
	if othersZero {
		placeholders := make([]string, 0, len(targets))
		args := make([]any, 0, len(targets))
		i := 1
		for name := range targets {
			placeholders = append(placeholders, fmt.Sprintf("$%d", i))
			args = append(args, name)
			i++
		}
		q := fmt.Sprintf(
			"UPDATE player SET is_wicket_keeper = 0 WHERE lower(player_name) NOT IN (%s)",
			strings.Join(placeholders, ","),
		)
		if _, err := r.DB.Exec(ctx, q, args...); err != nil {
			return fmt.Errorf("zero others: %w", err)
		}
	}
	return nil
}
