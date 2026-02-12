package weatherworker

import (
	"context"
	"errors"
	"log/slog"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/jobs"
)

// Service coordinates fetching weather for match IDs from a job source and
// upserting the resulting records via the Repository.
type Service struct {
	Jobs       jobs.Source
	Provider   Provider
	Repository Repository
}

func NewService(j jobs.Source, p Provider, r Repository) *Service {
	return &Service{Jobs: j, Provider: p, Repository: r}
}

// Run pulls batches from Jobs until exhausted or until maxJobs have been
// processed. When apply is false, it will not call Repository.
// Returns the number of match IDs processed or the first error encountered.
func (s *Service) Run(ctx context.Context, maxJobs int, apply bool) (int, error) {
	if s == nil || s.Jobs == nil || s.Provider == nil || s.Repository == nil {
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
			slog.Debug("fetching weather", slog.Int64("match_id", id))
			// Fetch all weather records for this match (e.g., batting/bowling sessions)
			recs, err := s.Provider.Fetch(ctx, id)
			if err != nil {
				return processed, err
			}
			if apply {
				slog.Info("upserting weather", slog.Int64("match_id", id), slog.Int("records", len(recs)))
				for _, r := range recs {
					if err := s.Repository.UpsertWeather(ctx, r); err != nil {
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
