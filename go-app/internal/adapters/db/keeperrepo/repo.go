package keeperrepo

import (
	"context"

	appdb "github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// Repo is a thin adapter delegating to internal/db keeper helpers.
type Repo struct{}

func New() *Repo { return &Repo{} }

// CountPlayersByLowerName returns count of players matching exact lower(name).
func (r *Repo) CountPlayersByLowerName(ctx context.Context, lowerName string) (int64, error) {
	return appdb.CountPlayersByLowerName(ctx, lowerName)
}

// SetIsWicketKeeperByLowerName updates the flag for an exact lower(name) match.
func (r *Repo) SetIsWicketKeeperByLowerName(ctx context.Context, value int, lowerName string) (int64, error) {
	return appdb.SetIsWicketKeeperByLowerName(ctx, value, lowerName)
}

// ZeroKeepersExcept sets is_wicket_keeper=0 where lower(name) NOT IN provided list.
func (r *Repo) ZeroKeepersExcept(ctx context.Context, lowerNames []string) (int64, error) {
	return appdb.ZeroKeepersExcept(ctx, lowerNames)
}
