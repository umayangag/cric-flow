package teamselect

import (
	"context"

	"github.com/umayangag/cric-flow/go-app/internal/selection"
)

// Selector abstracts team selection operations, enabling offline tests.
type Selector interface {
	SelectTeam(
		ctx context.Context,
		matchID int64,
		format string,
		opts selection.Options,
	) (selection.Result, error)
	SelectTeamFromCSV(
		ctx context.Context,
		poolPath string,
		matchID int64,
		format, season string,
		opts selection.Options,
	) (selection.Result, error)
}

type selectionAdapter struct{}

// NewSelectionAdapter returns a Selector that delegates to the selection package.
func NewSelectionAdapter() Selector { return selectionAdapter{} }

func (selectionAdapter) SelectTeam(
	ctx context.Context,
	matchID int64,
	format string,
	opts selection.Options,
) (selection.Result, error) {
	return selection.SelectTeam(ctx, matchID, format, opts)
}

func (selectionAdapter) SelectTeamFromCSV(
	ctx context.Context,
	poolPath string,
	matchID int64,
	format, season string,
	opts selection.Options,
) (selection.Result, error) {
	return selection.SelectTeamFromCSV(ctx, poolPath, matchID, format, season, opts)
}
