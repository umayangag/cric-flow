package importretired

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Runner orchestrates dry-run and apply flows for retired imports.
type Runner struct {
	DB DB
}

func NewRunner(db DB) *Runner { return &Runner{DB: db} }

// DryRun writes a deterministic summary of what would be applied.
func (r *Runner) DryRun(names []string, othersZero bool, out io.Writer) error {
	// Print header line matching legacy behavior
	if _, err := fmt.Fprintf(out, "[DRY-RUN] Would mark %d players as retired. others-zero=%v\n", len(names), othersZero); err != nil {
		return err
	}
	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			continue
		}
		if _, err := fmt.Fprintf(out, "  - %s\n", n); err != nil {
			return err
		}
	}
	if othersZero {
		_, _ = fmt.Fprintln(out, "[DRY-RUN] Would set is_retired=0 for all other players not listed")
	}
	return nil
}

// Apply executes updates for retired flags.
// Returns (changed, zeroed) counts, mirroring legacy behavior.
func (r *Runner) Apply(ctx context.Context, names []string, othersZero bool) (int64, int64, error) {
	slog.Info("starting retired players import", slog.Int("count", len(names)), slog.Bool("others_zero", othersZero))
	var changed int64

	if len(names) > 0 {
		namesLower := make([]string, len(names))
		for i, n := range names {
			namesLower[i] = strings.ToLower(n)
		}
		ct, err := r.DB.Exec(ctx, `UPDATE player SET is_retired = 1 WHERE lower(player_name) = ANY($1)`, namesLower)
		if err != nil {
			return 0, 0, fmt.Errorf("update retired players: %w", err)
		}
		changed = ct
		slog.Info("marked players as retired", slog.Int64("count", changed))
	}

	var zeroed int64
	if othersZero {
		if len(names) == 0 {
			ct, err := r.DB.Exec(ctx, `UPDATE player SET is_retired = 0 WHERE is_retired IS DISTINCT FROM 0`)
			if err != nil {
				return changed, zeroed, err
			}
			zeroed = ct
		} else {
			// Use a single text[] parameter to avoid dynamic SQL string formatting.
			namesLower := make([]string, len(names))
			for i, n := range names {
				namesLower[i] = strings.ToLower(n)
			}
			q := `UPDATE player
				SET is_retired = 0
				WHERE is_retired IS DISTINCT FROM 0
				AND NOT EXISTS (
					SELECT 1
					FROM unnest($1::text[]) AS t(name)
					WHERE lower(player_name) = t.name
				)`
			ct, err := r.DB.Exec(ctx, q, namesLower)
			if err != nil {
				return changed, zeroed, err
			}
			zeroed = ct
		}
	}
	return changed, zeroed, nil
}
