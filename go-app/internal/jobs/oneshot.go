package jobs

import (
	"context"
)

// OneShotSource is a jobs.Source implementation that yields a single match ID
// exactly once, then reports exhaustion.
type OneShotSource struct {
	id      int64
	yielded bool
}

// NewOneShotSource constructs a Source that will return the given id once.
func NewOneShotSource(id int64) *OneShotSource { return &OneShotSource{id: id} }

// Next returns the single id on the first call; after that it returns ok=false.
func (s *OneShotSource) Next(ctx context.Context, _ int) (ids []int64, ok bool, err error) {
	_ = ctx // no blocking work here
	if s == nil {
		return nil, false, nil
	}
	if s.yielded {
		return nil, false, nil
	}
	s.yielded = true
	return []int64{s.id}, true, nil
}
