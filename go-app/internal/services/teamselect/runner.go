package teamselect

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/selection"
)

// Runner orchestrates team selection by delegating to a Selector.
// It optionally connects to the DB via Connector when FromDB is true.
type Runner struct {
	Selector  Selector
	Connector db.Connector
}

func NewRunner(sel Selector, conn db.Connector) Runner { return Runner{Selector: sel, Connector: conn} }

// Run executes selection using either DB-backed features or a CSV pool and writes
// a deterministic summary to out.
func (r Runner) Run(ctx context.Context, opts Options, out io.Writer) error {
	w := out
	if w == nil {
		w = os.Stdout
	}
	var (
		res selection.Result
		err error
	)

	if opts.FromDB {
		if r.Connector != nil {
			if err := r.Connector.Connect(ctx); err != nil {
				return fmt.Errorf("db connect failed: %w", err)
			}
		}
		res, err = r.Selector.SelectTeam(ctx, opts.MatchID, opts.Format, selection.Options{
			TeamSize:      opts.TeamSize,
			MinBowlers:    opts.MinBowlers,
			RequireKeeper: opts.RequireKeeper,
		})
	} else {
		res, err = r.Selector.SelectTeamFromCSV(ctx, opts.PoolPath, selection.Options{
			TeamSize:      opts.TeamSize,
			MinBowlers:    opts.MinBowlers,
			RequireKeeper: opts.RequireKeeper,
		})
	}
	if err != nil {
		return err
	}
	// Print using the same format as previous implementation for parity.
	fmt.Fprintf(w, "Selected Team (size=%d) — Team Win Prob: %.4f\n", len(res.Players), res.TeamWinProbability)
	for i, p := range res.Players {
		fmt.Fprintf(w, "%2d. %s %.4f\n", i+1, p.PlayerName, p.WinningProbability)
	}
	return nil
}
