package importkeepers

import (
	"context"
	"fmt"
	"log/slog"
)

// KeeperRepository abstracts the minimal operations needed by this command.
type KeeperRepository interface {
	CountPlayersByLowerName(ctx context.Context, lowerName string) (int64, error)
	SetIsWicketKeeperByLowerName(ctx context.Context, value int, lowerName string) (int64, error)
	ZeroKeepersExcept(ctx context.Context, lowerNames []string) (int64, error)
}

// Runner orchestrates the import/preview flow using a repository.
type Runner struct {
	Repo KeeperRepository
}

func NewRunner(repo KeeperRepository) Runner { return Runner{Repo: repo} }

// Preview logs the planned changes without mutating the database.
func (r Runner) Preview(ctx context.Context, targets map[string]int, othersZero bool) error {
	matched := 0
	for name, v := range targets {
		cnt, err := r.Repo.CountPlayersByLowerName(ctx, name)
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
	slog.Info(
		"starting wicket-keeper status update",
		slog.Int("targets", len(targets)),
		slog.Bool("others_zero", othersZero),
	)
	for name, v := range targets {
		if _, err := r.Repo.SetIsWicketKeeperByLowerName(ctx, v, name); err != nil {
			return fmt.Errorf("update keeper for '%s': %w", name, err)
		}
		slog.Debug("updated wicket-keeper status", slog.String("name", name), slog.Int("value", v))
	}
	if othersZero {
		// Build list of lower-case names to exclude from zeroing.
		names := make([]string, 0, len(targets))
		for name := range targets {
			names = append(names, name)
		}
		if _, err := r.Repo.ZeroKeepersExcept(ctx, names); err != nil {
			return fmt.Errorf("zero others: %w", err)
		}
		slog.Info("zeroed wicket-keeper status for other players")
	}
	slog.Info("wicket-keeper status update finished")
	return nil
}
