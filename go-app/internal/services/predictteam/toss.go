package predictteam

import "fmt"

// Toss readings: which of the two quantities the probabilities in a response are (H-24).
//
// They are two different numbers, not two labels for one. On the served run the displayed
// probability for a TEST fixture moves 0.04 on average between them, and 0.14 at most, so
// a reader who cannot tell them apart cannot use either.
const (
	// TossReadingAware is the probability read at the batting order the caller named.
	TossReadingAware = "toss_aware"
	// TossReadingMarginalised is the probability averaged over both batting orders, which
	// is what the optimiser maximises and what the run manifest and the walk-forward
	// report score (EVAL-05).
	TossReadingMarginalised = "marginalised"
)

// TossReadings returns every value `toss.reading` may carry (H-24), for the surfaces that
// have to spell it.
func TossReadings() []string {
	return []string{TossReadingAware, TossReadingMarginalised}
}

// TossSummary says which batting order the numbers in a response were produced under.
//
// It is on the wire because the toss is now an input (P1-1) and §8.7 applies to inputs the
// same way it applies to fallbacks: a caller who names a toss and gets numbers drawn over
// both batting orders anyway has been answered a different question.
//
// `reading` names that question directly. It is not the request echoed back: it is derived
// from what each model reported having done, which is checked against what was asked for
// before this is built, so a caller does not have to re-read its own request — or know
// which models read a batting order — to know which of the two quantities it is holding.
type TossSummary struct {
	// Team1BatsFirst is nil when the toss was unknown and both batting orders were read.
	Team1BatsFirst *bool `json:"team1_bats_first"`
	// Reading is TossReadingAware or TossReadingMarginalised, and describes the headline
	// win probability and every per-player forecast beside it.
	Reading string `json:"reading"`
	// Note says what in this answer did *not* read the toss, where one was named. It is
	// absent on a marginalised answer, where there is nothing to carve out.
	Note string `json:"note,omitempty"`
}

// tossRead reports the reading the answer holds, from the toss that was asked for.
//
// Every model that reads a batting order is checked against this same toss before the
// result is assembled — a mismatch is an error, not a response — so the reading is a
// property of the answer rather than a restatement of the request.
func tossRead(team1BatsFirst *bool) TossSummary {
	if team1BatsFirst == nil {
		return TossSummary{Reading: TossReadingMarginalised}
	}
	return TossSummary{
		Team1BatsFirst: team1BatsFirst,
		Reading:        TossReadingAware,
		// The carve-out is real and is not a format's fault: the optimiser maximises the
		// XI-only objective, whose model reads per-side aggregates over eleven and has no
		// batting-order feature at all, and the constraint checks count roles. Both are
		// the same whichever side bats first, so the eleven on screen was chosen for the
		// average of both orders while the probability beside it was read at this one.
		Note: "The win probability and the per-player forecasts read this batting order. " +
			"The eleven was selected, and the constraint checks made, on the toss-blind " +
			"objective, which is the same either way.",
	}
}

// refuseTossMismatch refuses an answer whose batting order is not the one that was asked
// for. A known toss answered by a marginalised model — or the reverse — is the request
// being silently changed, and the `*_marginalised` flag is the one field that can catch it
// (§8.7). It is an error rather than a note because there is no honest label for a number
// produced under a batting order nobody asked about.
func refuseTossMismatch(step string, team1BatsFirst *bool, marginalised bool, field string) error {
	if marginalised == (team1BatsFirst == nil) {
		return nil
	}
	return fmt.Errorf("%s: the toss was %s but ml-service reports %s=%t",
		step, tossDescription(team1BatsFirst), field, marginalised)
}
