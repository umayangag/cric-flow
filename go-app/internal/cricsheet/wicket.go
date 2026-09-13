package cricsheet

import (
	"github.com/umayangag/cric-flow/go-app/internal/wicketkinds"
)

// WicketTally is what a delivery's wickets are to the scorecard: how many wickets the
// innings lost on it, and how many of those go into the bowler's column.
type WicketTally struct {
	// Dismissals is the innings' wickets lost on the delivery: every kind but a batter
	// who retired hurt or retired not out, who may come back and is not out.
	Dismissals int
	// CreditedToBowler is the part of Dismissals the bowler is credited with: bowled,
	// caught, caught and bowled, lbw, stumped and hit wicket. A run out, a retired out,
	// obstructing the field, handled the ball, hit the ball twice and timed out are
	// wickets the innings lost and nobody took.
	CreditedToBowler int
}

// TallyWickets classifies the delivery's wickets by the vocabulary. It is the one rule
// for bowling_data.wickets and match_inning.wickets_lost, and ml.xi.wicketkinds is the
// same rule -- the same file -- for the rating pass's wickets and dismissals targets, so
// a bowler's figures in the database and his target agree kind for kind. Until IMPORT-06
// was fixed every kind was credited to the bowler and every kind, retired hurt included,
// was a wicket lost. A kind the vocabulary does not know is an error, not a guess.
func (d Delivery) TallyWickets(vocabulary wicketkinds.Vocabulary) (WicketTally, error) {
	var tally WicketTally
	if d.Wickets == nil {
		return tally, nil
	}
	for _, w := range *d.Wickets {
		kind, err := vocabulary.Kind(w.Kind)
		if err != nil {
			return WicketTally{}, err
		}
		if kind.Class.IsDismissal() {
			tally.Dismissals++
		}
		if kind.Class == wicketkinds.CreditedToBowler {
			tally.CreditedToBowler++
		}
	}
	return tally, nil
}
