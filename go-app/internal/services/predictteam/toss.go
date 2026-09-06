package predictteam

// TossSummary says which batting order the numbers in a response were produced under.
//
// It is on the wire because the toss is now an input (P1-1) and §8.7 applies to inputs the
// same way it applies to fallbacks: a caller who names a toss and gets numbers drawn over
// both batting orders anyway has been answered a different question, and `toss_marginalised`
// alone cannot say which side the answer assumed batted first.
//
// Team1BatsFirst mirrors the request field exactly: nil is "unknown", which is the default
// and what the simulator has always done — half the draws each way.
type TossSummary struct {
	// Team1BatsFirst is nil when the toss was unknown and both batting orders were drawn.
	Team1BatsFirst *bool `json:"team1_bats_first"`
	// Honoured is false where the caller named a toss the forecast could not use.
	Honoured bool `json:"honoured"`
	// Note says why a named toss was not used, where one was not.
	Note string `json:"note,omitempty"`
}

// tossApplied reports the toss the simulator drew under: exactly what the caller asked for.
func tossApplied(team1BatsFirst *bool) TossSummary {
	return TossSummary{Team1BatsFirst: team1BatsFirst, Honoured: true}
}

// tossNotSimulated reports the toss on the path that has no innings to bat in.
//
// A format with no innings length is forecast per player rather than drawn as a match, so
// there is no batting order to fix. Naming a toss for one is a request this service cannot
// answer, and saying so beats returning numbers that quietly ignored it.
func tossNotSimulated(team1BatsFirst *bool) TossSummary {
	if team1BatsFirst == nil {
		return TossSummary{Honoured: true}
	}
	return TossSummary{
		Honoured: false,
		Note: "This format has no innings length, so there is no simulated match and no " +
			"batting order to fix: the toss you named was not used.",
	}
}
