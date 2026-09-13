// Package trackrecord scores the prediction record against what happened (P2-4).
//
// It is computed on read from the store (P2-3) and the match tables, with no column of its
// own, no pipeline step and no scheduler: an import that brings the match a prediction
// was about moves that prediction to "scored" by itself, because the next read finds the
// match. Everything here is arithmetic over rows somebody else wrote.
//
// What it is not: evidence. One operator's record holds tens of predictions, and a Brier
// or a reliability bin over tens is a diagnostic. Every number this package emits carries
// its n, none of them is a verdict, nothing is filtered by outcome, and a miss is the
// record working. `make evaluate` remains where a choice-facing number comes from; the
// surface shows the harness's figure beside the record's so a reader sees the record
// against the number the model was accepted on.
package trackrecord

import (
	"context"
	"time"
)

// The state every stored prediction is in -- exactly one. Declared in
// contracts/ops-console.contract.json and asserted from go-app and the frontend (H-24).
const (
	// StateScenario is a hand-built eleven (selection.objective "fixed"): listed and never
	// scored, because the caller built a side that may not have played.
	StateScenario = "scenario"
	// StateSuperseded is an Optimise forecast of a fixture that a later Optimise of the
	// same fixture, issued on or before the match date, replaced. One forecast is scored
	// per fixture, and it is the last one issued.
	StateSuperseded = "superseded"
	// StateUnresolved is a forecast whose match the database does not hold yet -- Cricsheet
	// lag, or an import not yet run. Shown with how many days past the match date it is.
	StateUnresolved = "unresolved"
	// StateNoResult is a forecast whose match was played with no winner recorded (no
	// result, tie or draw): counted, never scored.
	StateNoResult = "no_result"
	// StatePostHoc is a forecast issued after the day it was about, whose match was played
	// and won by someone. Its own score stays on the row -- what it claimed and what
	// happened are both on the record -- but it enters no summary: the ratings behind it
	// can already contain the result, so counting it would let hindsight flatter the
	// record instead of testing it (GO-03). Listed and counted, never aggregated.
	StatePostHoc = "post_hoc"
	// StateScored is a forecast issued on or before its match date whose match was played
	// and won by someone. These, and only these, are the rows every summary is over.
	StateScored = "scored"
)

// States is the vocabulary in one place, for the contract.
func States() []string {
	return []string{StateScenario, StateSuperseded, StateUnresolved, StateNoResult, StatePostHoc, StateScored}
}

// The simulator population a prediction's ranges belong to. Never pooled (B-12): a
// simulator without a shared match factor draws from a different model, and averaging
// its coverage with the factored one's averages two populations.
const (
	PopulationWithSharedFactor    = "with_shared_factor"
	PopulationWithoutSharedFactor = "without_shared_factor"
	// PopulationUnknown is a simulated answer stored before the store recorded which
	// simulator served it (migration 0014). Its own population, shown as such -- a guess
	// either way would be the pooling the split exists to prevent.
	PopulationUnknown = "unknown"
	// PopulationNotSimulated is an answer with no scorecard: a format with no innings
	// length. It has no ranges, so no coverage, and is counted here so it is not lost.
	PopulationNotSimulated = "not_simulated"
)

// Populations is the vocabulary in one place, for the contract.
func Populations() []string {
	return []string{
		PopulationWithSharedFactor, PopulationWithoutSharedFactor,
		PopulationUnknown, PopulationNotSimulated,
	}
}

// The two innings a coverage figure is reported for, as the harness names them
// (sim_harness._totals_report): by the order the innings were actually played in, not by
// which side the record called team1.
const (
	InningsFirst = "first_innings"
	InningsChase = "chase"
)

// MetricKeys are the L-1 glossary keys the record labels its numbers under. They are in
// the contract so ml-service's completeness gate can assert every one has an entry, and
// the frontend can assert it renders no other.
func MetricKeys() []string {
	return []string{"brier", "record_base_rate_brier", "reliability", "coverage_80", "eleven_overlap"}
}

// ReliabilityBins is the harness's RELIABILITY_BINS: ten equal-width probability bins.
const ReliabilityBins = 10

// Every opposition id this package handles -- on a stored prediction, on a Fixture and on
// a PlayedMatch -- is a club id: COALESCE(opposition.canonical_id, opposition.id), one id
// per club however many times it has been renamed. The store writes club ids because a
// prediction is filed under the side ResolveTeamSide resolved, and the lookup is required
// to answer in club ids for the same reason, so the comparisons below are like against
// like (GO-02). A lookup that answered in the raw ids of the match tables would leave a
// renamed club's fixture unresolved forever, and score the halves that did match the
// wrong way round.

// Fixture is the key a played match is matched on: the exact date, both clubs in either
// order, the format and the gender. A match on a neighbouring date is not it -- a series
// plays the same sides days apart, and the prediction was about one of those days.
type Fixture struct {
	Format    string
	Gender    string
	MatchDate time.Time
	Team1     int64
	Team2     int64
}

// PlayedMatch is what the database holds about a match the record scores against, with
// every side named by its club id.
type PlayedMatch struct {
	MatchID int64
	// WinnerOppositionID is the winning club, nil for a no-result, a tie or a draw.
	WinnerOppositionID *int64
	OutcomeByRuns      *int
	OutcomeByWickets   *int
	// Innings in the order they were played.
	Innings []PlayedInnings
	// FieldedPlayers is every player who took the field, keyed by player id, with the club
	// they played for.
	FieldedPlayers map[int64]int64
}

// PlayedInnings is one innings as match_inning holds it, batting club first.
type PlayedInnings struct {
	Number              int
	BattingOppositionID int64
	Runs                int
}

// MatchLookup finds the matches the database holds for a fixture. It returns every match
// that fits the key: none means unresolved, more than one means the record cannot say
// which was meant and says so rather than picking. Its answer is in club ids.
type MatchLookup interface {
	FindMatches(ctx context.Context, fixture Fixture) ([]PlayedMatch, error)
}

// --- the record on the wire ------------------------------------------------------------

// Record is the whole track record, computed on read.
type Record struct {
	ComputedAt string `json:"computed_at"`
	Today      string `json:"today"`
	Total      int    `json:"total"`
	// States counts every prediction by state; every state is present, zero included, so
	// a reader can see that nothing was dropped.
	States   map[string]int  `json:"states"`
	Win      WinSummary      `json:"win"`
	Coverage CoverageSummary `json:"coverage"`
	Elevens  ElevensSummary  `json:"elevens"`
	// Predictions is every stored prediction, newest first, in its state.
	Predictions []Entry `json:"predictions"`
}

// WinSummary scores the headline win probability of every scored prediction.
type WinSummary struct {
	Overall  WinScore            `json:"overall"`
	ByFormat map[string]WinScore `json:"by_format"`
	// Reliability is the harness's curve over the scored predictions: ten equal-width
	// bins, each with its n. Empty bins are omitted, as the harness omits them. An empty
	// record is an empty list, never an invented curve.
	Reliability     []ReliabilityBin `json:"reliability"`
	ReliabilityBins int              `json:"reliability_bins"`
}

// WinScore is a Brier with the base rate beside it, over n predictions.
type WinScore struct {
	N int `json:"n"`
	// Brier is the mean squared error of the served probability against the outcome.
	Brier *float64 `json:"brier"`
	// BaseRate is the scored predictions' own team1 win rate, and BaseRateBrier the Brier
	// of predicting it for every one of them: the score to beat, from the same rows.
	BaseRate      *float64 `json:"base_rate"`
	BaseRateBrier *float64 `json:"base_rate_brier"`
}

// ReliabilityBin is one of the harness's bins: mean predicted against observed frequency.
type ReliabilityBin struct {
	Lo        float64 `json:"lo"`
	Hi        float64 `json:"hi"`
	N         int     `json:"n"`
	Predicted float64 `json:"predicted"`
	Observed  float64 `json:"observed"`
}

// CoverageSummary is the served 10-90 totals against the actual innings totals, per
// format and simulator population, per innings. There is no pooled row: a figure over two
// populations would be the average of two simulators (B-12).
type CoverageSummary struct {
	Rows []CoverageRow `json:"rows"`
	// Populations counts the scored predictions in each population, every population
	// present, so the denominators are visible even where a population has no ranges.
	Populations map[string]int `json:"populations"`
}

// CoverageRow is one (format, population) pair with both innings' coverage.
type CoverageRow struct {
	Format       string        `json:"format"`
	Population   string        `json:"population"`
	NPredictions int           `json:"n_predictions"`
	FirstInnings CoverageScore `json:"first_innings"`
	Chase        CoverageScore `json:"chase"`
}

// CoverageScore is how many of n actual totals fell inside their served 10-90 range.
type CoverageScore struct {
	N        int      `json:"n"`
	Covered  int      `json:"covered"`
	Coverage *float64 `json:"coverage"`
}

// ElevensSummary is how many of the predicted players took the field, over the scored
// predictions. Reported beside the scores and never used to exclude one.
type ElevensSummary struct {
	N    int      `json:"n"`
	Mean *float64 `json:"mean_overlap"`
	Min  *int     `json:"min_overlap"`
	Max  *int     `json:"max_overlap"`
	// Complete counts the predictions whose every named player played.
	Complete int `json:"complete"`
}

// Entry is one stored prediction in its state: what was claimed, what happened, and the
// score where there is one.
type Entry struct {
	ID             string `json:"id"`
	IssuedAt       string `json:"issued_at"`
	RunID          string `json:"run_id"`
	RatingsThrough string `json:"ratings_through"`
	Format         string `json:"format"`
	Gender         string `json:"gender"`
	MatchDate      string `json:"match_date"`
	Team1          Side   `json:"team1"`
	Team2          Side   `json:"team2"`
	Objective      string `json:"objective"`
	State          string `json:"state"`
	// StateNote says why, where the state needs saying: which forecast superseded this
	// one, or why a resolved-looking fixture is still unresolved.
	StateNote    string `json:"state_note,omitempty"`
	SupersededBy string `json:"superseded_by,omitempty"`
	// DaysPastMatchDate is set on an unresolved prediction: negative before the match.
	DaysPastMatchDate *int `json:"days_past_match_date,omitempty"`
	// IssuedAfterMatchDate flags a forecast issued after the day it was about. Such a
	// forecast is in StatePostHoc: its score is on the row, and no summary is over it
	// (§8.7, GO-03).
	IssuedAfterMatchDate bool      `json:"issued_after_match_date,omitempty"`
	Population           string    `json:"population"`
	Claimed              Claimed   `json:"claimed"`
	Happened             *Happened `json:"happened,omitempty"`
	Score                *Score    `json:"score,omitempty"`
	// PayloadError names a stored payload this record could not read. The row stays on
	// the record in its state; only the parts read out of the payload are missing.
	PayloadError string `json:"payload_error,omitempty"`
}

// Side is one of the two teams, as the payload named it.
type Side struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Claimed is what the prediction said.
type Claimed struct {
	WinProbabilityTeam1  float64 `json:"win_probability_team1"`
	WinProbabilitySource string  `json:"win_probability_source"`
	PredictedWinnerID    int64   `json:"predicted_winner_id"`
	// Team1Range and Team2Range are the served 10-90 totals, as served: no widening, no
	// day/night adjustment (B-11 is open and the record shows it). Absent where no
	// simulator ran.
	Team1Range *Range `json:"team1_range,omitempty"`
	Team2Range *Range `json:"team2_range,omitempty"`
	// Players named, per side.
	Team1Players int `json:"team1_players"`
	Team2Players int `json:"team2_players"`
}

// Range is a served 10-90 interval.
type Range struct {
	P10 float64 `json:"p10"`
	P90 float64 `json:"p90"`
}

// Happened is what the database holds about the match, once it does.
type Happened struct {
	MatchID            int64  `json:"match_id"`
	WinnerOppositionID *int64 `json:"winner_opposition_id"`
	OutcomeByRuns      *int   `json:"outcome_by_runs,omitempty"`
	OutcomeByWickets   *int   `json:"outcome_by_wickets,omitempty"`
	Team1Total         *int   `json:"team1_total"`
	Team2Total         *int   `json:"team2_total"`
	Team1BattedFirst   *bool  `json:"team1_batted_first"`
}

// Score is one scored prediction against its match.
type Score struct {
	Team1Won bool    `json:"team1_won"`
	Brier    float64 `json:"brier"`
	// Team1Covered and Team2Covered say whether each side's actual total fell inside its
	// served range; nil where there was no range, or no innings for that side.
	Team1Covered  *bool         `json:"team1_covered"`
	Team2Covered  *bool         `json:"team2_covered"`
	ElevenOverlap ElevenOverlap `json:"eleven_overlap"`
}

// ElevenOverlap is how many of the named players took the field for the side they were
// named for.
type ElevenOverlap struct {
	Matched      int `json:"matched"`
	Of           int `json:"of"`
	Team1Matched int `json:"team1_matched"`
	Team2Matched int `json:"team2_matched"`
}
