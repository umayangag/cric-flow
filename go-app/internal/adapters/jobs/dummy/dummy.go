package dummy

import (
	"context"
)

// Source is a trivial in-memory jobs source for smoke/wiring.
// It returns match IDs from the provided slice in FIFO order.

type Source struct {
	ids []int64
	pos int
}

func New(ids []int64) *Source { return &Source{ids: append([]int64(nil), ids...)} }

func (s *Source) Next(_ context.Context, batch int) ([]int64, bool, error) {
	if s.pos >= len(s.ids) {
		return nil, false, nil
	}
	end := s.pos + batch
	if end > len(s.ids) {
		end = len(s.ids)
	}
	out := s.ids[s.pos:end]
	s.pos = end
	return out, true, nil
}
