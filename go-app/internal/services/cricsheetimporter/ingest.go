package cricsheetimporter

import (
	"context"
	"errors"
	"runtime"
	"sync"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/cricsheet"
	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// IngestService coordinates loading Cricsheet files, parsing them into matches,
// and (optionally) persisting them via the repository. It is designed for
// deterministic, unit-testable behavior: concurrency is bounded, and errors
// stop processing promptly.
//
// Dependencies are small interfaces so tests can supply fakes or mockery mocks.
//go:generate mockery --name IngestService --output internal/mocks --case underscore
// NOTE: We do not usually mock the service itself; mocks are generated primarily
// for Loader/Parser/Repo elsewhere. The directive above is a convenience if
// other packages need to mock this service.
//
// The service is independent from the command Runner so it can be tested in
// isolation and reused if needed.

type IngestService struct {
	Loader cricsheet.Loader
	Parser cricsheet.Parser
	Repo   db.MatchRepo
}

// IngestDir processes all inputs returned by Loader.List for the given dir.
//
//   - If apply == false, it will not write to Repo and simply validates it can
//     load+parse all inputs.
//   - concurrency <= 0 defaults to 1; otherwise spawns up to `concurrency` workers.
//
// Returns the count of files successfully processed (loaded+parsed; and upserted
// when apply == true) or the first error encountered.
func (s *IngestService) IngestDir(ctx context.Context, dir string, apply bool, concurrency int) (int, error) {
	if s == nil || s.Loader == nil || s.Parser == nil || s.Repo == nil {
		return 0, errors.New("nil service or dependency")
	}
	if dir == "" {
		return 0, errors.New("input directory required")
	}

	ids, err := s.Loader.List(ctx, dir)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	// Upper bound to something sensible so tests stay deterministic even if a
	// huge number is passed.
	if concurrency > runtime.NumCPU()*4 {
		concurrency = runtime.NumCPU() * 4
	}

	type result struct {
		idx int
		err error
	}

	jobs := make(chan int)
	resC := make(chan result)
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for w := 0; w < concurrency; w++ {
		go func() {
			defer wg.Done()
			for idx := range jobs {
				id := ids[idx]
				raw, e := s.Loader.Load(ctx, dir, id)
				if e != nil {
					resC <- result{idx: idx, err: e}
					return
				}
				matches, e := s.Parser.Parse(ctx, raw)
				if e != nil {
					resC <- result{idx: idx, err: e}
					return
				}
				if apply {
					if e = s.Repo.UpsertMatches(ctx, matches); e != nil {
						resC <- result{idx: idx, err: e}
						return
					}
				}
				resC <- result{idx: idx, err: nil}
			}
		}()
	}

	go func() {
		for i := range ids {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(resC)
	}()

	processed := 0
	for r := range resC {
		if r.err != nil {
			return processed, r.err
		}
		processed++
	}
	return processed, nil
}
