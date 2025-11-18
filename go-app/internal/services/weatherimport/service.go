package weatherimport

import (
	"context"
	"errors"
)

// Service coordinates fetching weather from a Provider and (optionally) upserting via Repo.
type Service struct {
	Provider Provider
	Repo     Repository
}

func NewService(p Provider, r Repository) *Service { return &Service{Provider: p, Repo: r} }

// Import fetches all weather records for a match and upserts them when apply==true.
// Returns the number of records processed or an error.
func (s *Service) Import(ctx context.Context, matchID int64, apply bool) (int, error) {
	if s == nil || s.Provider == nil || s.Repo == nil {
		return 0, errors.New("nil service or dependency")
	}
	if matchID <= 0 {
		return 0, errors.New("invalid match id")
	}
	recs, err := s.Provider.Fetch(ctx, matchID)
	if err != nil {
		return 0, err
	}
	if !apply {
		return len(recs), nil
	}
	count := 0
	for _, r := range recs {
		if err := s.Repo.UpsertWeather(ctx, r); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
