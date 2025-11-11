package weatherworker

import (
	"context"
	"errors"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/jobs"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/wx"
)

// Service coordinates fetching weather for match IDs from a job source and,
// when apply==true, upserting the resulting records via the WeatherRepo.
// It is deterministic and testable; no logging here.
//
type Service struct {
	Jobs jobs.Source
	Prov wx.Provider
	Repo db.WeatherRepo
}

func NewService(j jobs.Source, p wx.Provider, r db.WeatherRepo) *Service { return &Service{Jobs: j, Prov: p, Repo: r} }

// Run pulls batches from Jobs until exhausted or until maxJobs have been
// processed. When apply is false, it will not call Repo.
// Returns the number of match IDs processed or the first error encountered.
func (s *Service) Run(ctx context.Context, maxJobs int, apply bool) (int, error) {
	if s == nil || s.Jobs == nil || s.Prov == nil || s.Repo == nil {
		return 0, errors.New("nil service or dependency")
	}
	if maxJobs < 0 {
		return 0, errors.New("maxJobs must be >= 0")
	}
	processed := 0
	remaining := maxJobs
	// Choose a reasonable batch size to keep loops efficient.
	const batchSize = 16
	for {
		// Respect ctx cancelation between pulls.
		select {
		case <-ctx.Done():
			return processed, ctx.Err()
		default:
		}
		ids, ok, err := s.Jobs.Next(ctx, batchSize)
		if err != nil {
			return processed, err
		}
		if !ok || len(ids) == 0 {
			return processed, nil
		}
		for _, id := range ids {
			if maxJobs > 0 && remaining == 0 {
				return processed, nil
			}
			// Fetch all weather records for this match (e.g., batting/bowling sessions)
			recs, err := s.Prov.Fetch(ctx, id)
			if err != nil {
				return processed, err
			}
			if apply {
				for _, r := range recs {
					if err := s.Repo.UpsertWeather(ctx, r); err != nil {
						return processed, err
					}
				}
			}
			processed++
			if maxJobs > 0 {
				remaining--
			}
		}
	}
}
