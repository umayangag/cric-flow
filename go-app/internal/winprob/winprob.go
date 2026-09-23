// Package winprob holds the one rule for turning a team1 win probability into a named
// winner (GO-10). Before this package existed the rule was written out twice — the live
// prediction path used `>= 0.5` for team1, the track record used `> 0.5` — and each site's
// comment claimed the other's threshold, so neither the code nor the docs told a reader
// which one actually ran. Centralising it here makes drifting apart again a compile error:
// there is exactly one place the tie is broken.
package winprob

// Team1Wins reports whether team1WinProbability names team1 as the predicted winner.
//
// The tie at exactly 0.5 goes to team1. That is the rule the live serving path
// (predictteam.winnerFrom) already used, so choosing it here keeps today's served answers
// unchanged; the track record is the side being brought into line, not the other way round,
// because it is a read of history rather than an answer already handed to a caller.
func Team1Wins(team1WinProbability float64) bool {
	return team1WinProbability >= 0.5
}
