// Package selectionbacktest compares ways of picking an XI over matches that have
// already been played.
//
// # What this can and cannot measure
//
// The question everyone wants answered is "would the optimiser's XI have won more
// often?" That question is not answerable from historical data. The match was played by
// the teams that were actually fielded; replaying it with a different XI would need a
// simulator whose accuracy is exactly what is in doubt. Any metric claiming otherwise
// would be the simulator's opinion of itself.
//
// So this package measures three things that *are* observable, and says which is which:
//
//   - **Winner accuracy.** How often the arm's predicted winner matches the real result.
//     This scores the win model and the selection jointly, against ground truth. It is
//     the only metric here grounded in what actually happened.
//   - **Mean predicted win probability.** How far an arm moves its own objective. An
//     internal consistency check: it says the search is working, not that it is right.
//     An arm can win this and be worse in reality.
//   - **Divergence between arms.** How many players two arms choose differently. If the
//     optimiser returns the greedy XI, the search is not doing anything, and no amount
//     of the other two numbers changes that.
//
// Overlap with the XI that was actually fielded is reported for context only. Real
// selectors are not optimal, so agreeing with them is not evidence of being right.
package selectionbacktest

import (
	"context"
	"sort"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// Arm is one selection strategy under comparison.
type Arm struct {
	// Name labels the arm in the report.
	Name string
	// Mode is the selection mode passed through to the prediction path.
	Mode predictteam.SelectionMode
}

// Match is one already-played fixture to select for.
type Match struct {
	MatchID   int64
	Format    string
	Team1     string
	Team2     string
	MatchDate time.Time
	// ActualWinner is the team name that won, empty when the match had no result.
	ActualWinner string
	// FieldedPlayerIDs is who actually played, both sides. Empty when unknown.
	FieldedPlayerIDs []int64
}

// ArmSelection is what one arm chose for one match.
type ArmSelection struct {
	// Team1WinProbability is the win model's read on the XIs this arm picked.
	Team1WinProbability float64
	// PredictedWinner is a team name, empty when the model expressed no preference.
	PredictedWinner string
	// SelectedPlayerIDs is both selected XIs.
	SelectedPlayerIDs []int64
}

// Selector runs one arm for one match. Implemented over the real prediction path by
// the CLI; a function type so the comparison logic can be tested without a database,
// an ML service, or a trained model.
type Selector func(ctx context.Context, match Match, mode predictteam.SelectionMode) (ArmSelection, error)

// Selection is one arm's result for one match, or the reason there is none.
type Selection struct {
	MatchID   int64
	Arm       string
	Selection ArmSelection
	Match     Match
	// Err is set when this match could not be selected for this arm. A backtest that
	// abandoned hours of work because one match had no squad would be useless, so a
	// failure is recorded against the arm and counted rather than returned.
	Err error
}

// Run selects every match under every arm.
//
// Arms are run per match rather than per arm so that a run stopped halfway still holds
// a comparable sample: every match it reached was selected both ways.
func Run(ctx context.Context, matches []Match, arms []Arm, selector Selector) []Selection {
	out := make([]Selection, 0, len(matches)*len(arms))
	for _, match := range matches {
		if ctx.Err() != nil {
			return out
		}
		for _, arm := range arms {
			selection, err := selector(ctx, match, arm.Mode)
			out = append(out, Selection{
				MatchID:   match.MatchID,
				Arm:       arm.Name,
				Selection: selection,
				Match:     match,
				Err:       err,
			})
		}
	}
	return out
}

// ArmReport is one arm's aggregate over the matches it selected.
type ArmReport struct {
	Arm string `json:"arm"`
	// Matches is how many selections succeeded; Failed is how many did not.
	Matches int `json:"matches"`
	Failed  int `json:"failed"`
	// MeanTeam1WinProbability is the arm's own objective, averaged. It says the search
	// moved, not that it moved somewhere true.
	MeanTeam1WinProbability *float64 `json:"mean_team1_win_probability"`
	// WinnerAccuracy is the share of matches whose predicted winner was the real one,
	// over matches that had both a prediction and a result. Nil when there were none.
	WinnerAccuracy *float64 `json:"winner_accuracy"`
	// DecidedMatches is the denominator behind WinnerAccuracy, reported because an
	// accuracy over four matches and one over four hundred should not read alike.
	DecidedMatches int `json:"decided_matches"`
	// MeanFieldedOverlap is the mean share of the actually-fielded players this arm
	// also picked. Context only -- real selectors are not optimal.
	MeanFieldedOverlap *float64 `json:"mean_fielded_overlap"`
}

// Divergence is how differently two arms chose.
type Divergence struct {
	ArmA string `json:"arm_a"`
	ArmB string `json:"arm_b"`
	// Matches is how many matches both arms selected successfully.
	Matches int `json:"matches"`
	// IdenticalXIs is how often they picked exactly the same players. When this equals
	// Matches, the second arm is not doing anything the first did not already do.
	IdenticalXIs int `json:"identical_xis"`
	// MeanDifferentPlayers is the average count of players in one selection and not the
	// other, per match.
	MeanDifferentPlayers float64 `json:"mean_different_players"`
}

// Report is the whole comparison.
type Report struct {
	Matches     int          `json:"matches"`
	Arms        []ArmReport  `json:"arms"`
	Divergences []Divergence `json:"divergences"`
}

// Summarize aggregates raw selections into the report.
func Summarize(selections []Selection) Report {
	byArm := map[string][]Selection{}
	armOrder := []string{}
	matchIDs := map[int64]bool{}
	for _, s := range selections {
		if _, seen := byArm[s.Arm]; !seen {
			armOrder = append(armOrder, s.Arm)
		}
		byArm[s.Arm] = append(byArm[s.Arm], s)
		matchIDs[s.MatchID] = true
	}

	report := Report{Matches: len(matchIDs)}
	for _, arm := range armOrder {
		report.Arms = append(report.Arms, summarizeArm(arm, byArm[arm]))
	}
	for i := 0; i < len(armOrder); i++ {
		for j := i + 1; j < len(armOrder); j++ {
			report.Divergences = append(report.Divergences, compareArms(byArm, armOrder[i], armOrder[j]))
		}
	}
	return report
}

func summarizeArm(arm string, selections []Selection) ArmReport {
	out := ArmReport{Arm: arm}
	var probabilitySum, overlapSum float64
	var overlapCount, correctWinners int

	for _, s := range selections {
		if s.Err != nil {
			out.Failed++
			continue
		}
		out.Matches++
		probabilitySum += s.Selection.Team1WinProbability

		if s.Match.ActualWinner != "" && s.Selection.PredictedWinner != "" {
			out.DecidedMatches++
			if s.Selection.PredictedWinner == s.Match.ActualWinner {
				correctWinners++
			}
		}
		if len(s.Match.FieldedPlayerIDs) > 0 {
			overlapCount++
			shared := overlap(s.Selection.SelectedPlayerIDs, s.Match.FieldedPlayerIDs)
			overlapSum += float64(shared) / float64(len(s.Match.FieldedPlayerIDs))
		}
	}

	if out.Matches > 0 {
		out.MeanTeam1WinProbability = ratio(probabilitySum, float64(out.Matches))
	}
	if out.DecidedMatches > 0 {
		out.WinnerAccuracy = ratio(float64(correctWinners), float64(out.DecidedMatches))
	}
	if overlapCount > 0 {
		out.MeanFieldedOverlap = ratio(overlapSum, float64(overlapCount))
	}
	return out
}

func compareArms(byArm map[string][]Selection, armA, armB string) Divergence {
	out := Divergence{ArmA: armA, ArmB: armB}
	byMatchA := successfulByMatch(byArm[armA])
	byMatchB := successfulByMatch(byArm[armB])

	var differentSum int
	for matchID, a := range byMatchA {
		b, ok := byMatchB[matchID]
		if !ok {
			continue
		}
		out.Matches++
		different := symmetricDifference(a.Selection.SelectedPlayerIDs, b.Selection.SelectedPlayerIDs)
		differentSum += different
		if different == 0 {
			out.IdenticalXIs++
		}
	}
	if out.Matches > 0 {
		out.MeanDifferentPlayers = float64(differentSum) / float64(out.Matches)
	}
	return out
}

func successfulByMatch(selections []Selection) map[int64]Selection {
	out := make(map[int64]Selection, len(selections))
	for _, s := range selections {
		if s.Err == nil {
			out[s.MatchID] = s
		}
	}
	return out
}

// overlap counts the ids present in both slices, ignoring duplicates.
func overlap(a, b []int64) int {
	set := make(map[int64]bool, len(a))
	for _, id := range a {
		set[id] = true
	}
	seen := make(map[int64]bool, len(b))
	count := 0
	for _, id := range b {
		if set[id] && !seen[id] {
			seen[id] = true
			count++
		}
	}
	return count
}

// symmetricDifference counts ids in one slice and not the other, both directions.
func symmetricDifference(a, b []int64) int {
	setA := make(map[int64]bool, len(a))
	for _, id := range a {
		setA[id] = true
	}
	setB := make(map[int64]bool, len(b))
	for _, id := range b {
		setB[id] = true
	}
	count := 0
	for id := range setA {
		if !setB[id] {
			count++
		}
	}
	for id := range setB {
		if !setA[id] {
			count++
		}
	}
	return count
}

func ratio(numerator, denominator float64) *float64 {
	if denominator == 0 {
		return nil
	}
	value := numerator / denominator
	return &value
}

// SortedArmNames returns the arm names in the report, sorted, for stable rendering.
func SortedArmNames(report Report) []string {
	names := make([]string, 0, len(report.Arms))
	for _, a := range report.Arms {
		names = append(names, a.Arm)
	}
	sort.Strings(names)
	return names
}
