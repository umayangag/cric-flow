package fielding

import (
	"context"
	"errors"
	"sync"

	"github.com/umayangag/cric-info-scrapers/go-app/internal/db"
)

// Service aggregates fielding events into per-player per-match totals and upserts them via Repo.
type Service struct {
	Repo db.FieldingRepo
}

func NewService(repo db.FieldingRepo) *Service { return &Service{Repo: repo} }

// BackfillMatch aggregates a single match and writes results when apply=true.
func (s *Service) BackfillMatch(ctx context.Context, matchID int64, apply bool) (int, error) {
	if s == nil || s.Repo == nil {
		return 0, errors.New("nil service or repo")
	}
	if matchID <= 0 {
		return 0, errors.New("invalid match id")
	}
	mid := matchID
	events, err := s.Repo.ListFieldingEvents(ctx, &mid)
	if err != nil {
		return 0, err
	}
	aggs := aggregate(events)
	rows := aggs[mid]
	if !apply {
		return len(rows), nil
	}
	if err := s.Repo.UpsertFieldingAggregates(ctx, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// BackfillAll aggregates all matches. Concurrency applies to the upsert step per match batch.
func (s *Service) BackfillAll(ctx context.Context, apply bool, concurrency int) (int, error) {
	if s == nil || s.Repo == nil {
		return 0, errors.New("nil service or repo")
	}
	if concurrency < 1 {
		return 0, errors.New("concurrency must be >= 1")
	}
	events, err := s.Repo.ListFieldingEvents(ctx, nil)
	if err != nil {
		return 0, err
	}
	aggs := aggregate(events)
	// count total rows
	total := 0
	for _, rows := range aggs { total += len(rows) }
	if !apply { return total, nil }

	// upsert per match with simple worker pool
 type task struct{ rows []db.FieldingAggregateRow }
	jobs := make(chan task)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	worker := func() {
		defer wg.Done()
		for t := range jobs {
			if err := s.Repo.UpsertFieldingAggregates(ctx, t.rows); err != nil {
				mu.Lock(); if firstErr == nil { firstErr = err }; mu.Unlock(); return
			}
		}
	}
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ { go worker() }
	for _, rows := range aggs {
		// If an error already occurred, stop scheduling further work
		mu.Lock(); err := firstErr; mu.Unlock(); if err != nil { break }
		jobs <- task{rows: rows}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil { return 0, firstErr }
	return total, nil
}

// aggregate groups events by match and player.
func aggregate(events []db.BackfillEvent) map[int64][]db.FieldingAggregateRow {
	// key: match -> (player -> aggregate)
	byMatch := map[int64]map[int64]*db.FieldingAggregateRow{}
	for _, e := range events {
		m := byMatch[e.MatchID]
		if m == nil { m = map[int64]*db.FieldingAggregateRow{}; byMatch[e.MatchID] = m }
		ag := m[e.PlayerID]
		if ag == nil {
			ag = &db.FieldingAggregateRow{MatchID: e.MatchID, PlayerID: e.PlayerID}
			m[e.PlayerID] = ag
		}
		ag.Catches += e.Catches
		ag.RunOuts += e.RunOuts
		ag.Stumpings += e.Stumpings
		ag.RunoutsDirectHits += e.RunoutsDirectHits
	}
	out := make(map[int64][]db.FieldingAggregateRow, len(byMatch))
	for mid, players := range byMatch {
		rows := make([]db.FieldingAggregateRow, 0, len(players))
		for _, ag := range players { rows = append(rows, *ag) }
		out[mid] = rows
	}
	return out
}
