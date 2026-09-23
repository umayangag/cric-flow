package trackrecord

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/predictions"
	"github.com/umayangag/cric-flow/go-app/internal/winprob"
)

// The objective a hand-built eleven is stored under (contract: selection_objectives).
// It is the one value that separates a scenario from a forecast about a fixture.
const objectiveFixed = "fixed"

// Build computes the record from every stored prediction and the matches the lookup can
// find for them, as of `today`.
//
// Nothing here writes. The states fall out of the store and the match tables on every
// read, which is what lets an import resolve a prediction with no operator step.
func Build(
	ctx context.Context,
	stored []predictions.Prediction,
	lookup MatchLookup,
	today time.Time,
) (*Record, error) {
	entries := make([]*entry, 0, len(stored))
	for i := range stored {
		entries = append(entries, newEntry(stored[i]))
	}
	markSuperseded(entries)

	resolved, err := resolveFixtures(ctx, entries, lookup)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		e.resolve(resolved, today)
	}

	record := &Record{
		ComputedAt:  time.Now().UTC().Format(time.RFC3339),
		Today:       today.Format(time.DateOnly),
		Total:       len(entries),
		States:      countStates(entries),
		Predictions: make([]Entry, 0, len(entries)),
	}
	record.Win = summariseWins(entries)
	record.Coverage = summariseCoverage(entries)
	record.Elevens = summariseElevens(entries)

	// Newest first: the record is read from the most recent answer back.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].stored.IssuedAt.Equal(entries[j].stored.IssuedAt) {
			return entries[i].stored.ID > entries[j].stored.ID
		}
		return entries[i].stored.IssuedAt.After(entries[j].stored.IssuedAt)
	})
	for _, e := range entries {
		record.Predictions = append(record.Predictions, e.Entry)
	}
	return record, nil
}

// entry is one prediction while the record is being built: the wire entry, the stored
// row it came from and the payload read out of it.
type entry struct {
	Entry
	stored  predictions.Prediction
	payload *storedPayload
	// p and y are the scored prediction's probability and outcome, for the summaries.
	p, y float64
}

func newEntry(stored predictions.Prediction) *entry {
	e := &entry{
		stored: stored,
		Entry: Entry{
			ID:             stored.ID,
			IssuedAt:       stored.IssuedAt.UTC().Format(time.RFC3339),
			RunID:          stored.RunID,
			RatingsThrough: stored.RatingsThrough.Format(time.DateOnly),
			Format:         stored.FormatCode,
			Gender:         stored.Gender,
			MatchDate:      stored.MatchDate.Format(time.DateOnly),
			Team1:          Side{ID: stored.Team1OppositionID},
			Team2:          Side{ID: stored.Team2OppositionID},
			Objective:      stored.SelectionObjective,
			Claimed: Claimed{
				WinProbabilityTeam1:  stored.WinProbabilityTeam1,
				WinProbabilitySource: stored.WinProbabilitySource,
				PredictedWinnerID:    predictedWinner(stored),
			},
		},
	}
	payload, err := readPayload(stored.Payload)
	if err != nil {
		e.PayloadError = err.Error()
		e.Population = population(stored, nil)
		return e
	}
	e.payload = payload
	e.Team1.Name = payload.Team1Side.DisplayName
	e.Team2.Name = payload.Team2Side.DisplayName
	e.Claimed.Team1Players = len(payload.Team1)
	e.Claimed.Team2Players = len(payload.Team2)
	if payload.Scorecard != nil {
		e.Claimed.Team1Range = &Range{P10: payload.Scorecard.Team1Innings.P10, P90: payload.Scorecard.Team1Innings.P90}
		e.Claimed.Team2Range = &Range{P10: payload.Scorecard.Team2Innings.P10, P90: payload.Scorecard.Team2Innings.P90}
	}
	e.Population = population(stored, payload)
	return e
}

// predictedWinner is the side the headline probability favoured, using the same tie-break
// rule the serving path used to answer it (winprob.Team1Wins, GO-10) -- so a fixture whose
// stored probability landed at exactly 0.5 is scored against the winner it was actually
// served as, not a second, disagreeing opinion computed here.
func predictedWinner(stored predictions.Prediction) int64 {
	if winprob.Team1Wins(stored.WinProbabilityTeam1) {
		return stored.Team1OppositionID
	}
	return stored.Team2OppositionID
}

// population names the simulator that served the answer. The column decides where it
// is set; where it is nil the payload says whether a simulator ran at all, and a
// simulated answer with no column is the third population, "unknown".
func population(stored predictions.Prediction, payload *storedPayload) string {
	if stored.SimulatorSharedFactor != nil {
		if *stored.SimulatorSharedFactor {
			return PopulationWithSharedFactor
		}
		return PopulationWithoutSharedFactor
	}
	if payload == nil || payload.Scorecard == nil {
		return PopulationNotSimulated
	}
	return PopulationUnknown
}

func (e *entry) isScenario() bool { return e.stored.SelectionObjective == objectiveFixed }

// issuedOnOrBeforeMatchDate is the superseding condition: a forecast issued on the match
// day still counts as before the match; one issued after the day it was about does not
// supersede anything and stands on its own, flagged.
func (e *entry) issuedOnOrBeforeMatchDate() bool {
	issued := e.stored.IssuedAt.UTC()
	issuedDay := time.Date(issued.Year(), issued.Month(), issued.Day(), 0, 0, 0, 0, time.UTC)
	return !issuedDay.After(e.stored.MatchDate)
}

func (e *entry) fixture() Fixture {
	return Fixture{
		Format:    e.stored.FormatCode,
		Gender:    e.stored.Gender,
		MatchDate: e.stored.MatchDate,
		Team1:     e.stored.Team1OppositionID,
		Team2:     e.stored.Team2OppositionID,
	}
}

// fixtureKey identifies a fixture regardless of which side was named first.
type fixtureKey struct {
	format, gender, date string
	low, high            int64
}

func keyOf(f Fixture) fixtureKey {
	low, high := f.Team1, f.Team2
	if low > high {
		low, high = high, low
	}
	return fixtureKey{f.Format, f.Gender, f.MatchDate.Format(time.DateOnly), low, high}
}

// markSuperseded applies the one-forecast-per-fixture rule: among the Optimise forecasts
// of a fixture issued on or before its match date, the last one issued stands and the
// rest are superseded by it. Scenarios take no part; forecasts issued after the match
// date take no part either, in both directions.
func markSuperseded(entries []*entry) {
	latest := map[fixtureKey]*entry{}
	for _, e := range entries {
		if e.isScenario() || !e.issuedOnOrBeforeMatchDate() {
			continue
		}
		key := keyOf(e.fixture())
		current, seen := latest[key]
		if !seen || e.stored.IssuedAt.After(current.stored.IssuedAt) ||
			(e.stored.IssuedAt.Equal(current.stored.IssuedAt) && e.stored.ID > current.stored.ID) {
			latest[key] = e
		}
	}
	for _, e := range entries {
		if e.isScenario() || !e.issuedOnOrBeforeMatchDate() {
			continue
		}
		winner := latest[keyOf(e.fixture())]
		if winner != e {
			e.State = StateSuperseded
			e.SupersededBy = winner.ID
			e.StateNote = "a later forecast of the same fixture was issued " + winner.IssuedAt
		}
	}
}

// resolveFixtures asks the lookup once per distinct fixture that still needs a match.
func resolveFixtures(
	ctx context.Context,
	entries []*entry,
	lookup MatchLookup,
) (map[fixtureKey][]PlayedMatch, error) {
	resolved := map[fixtureKey][]PlayedMatch{}
	for _, e := range entries {
		if e.isScenario() || e.State == StateSuperseded {
			continue
		}
		key := keyOf(e.fixture())
		if _, done := resolved[key]; done {
			continue
		}
		matches, err := lookup.FindMatches(ctx, e.fixture())
		if err != nil {
			return nil, fmt.Errorf("resolve %s %s v %s on %s: %w",
				e.Format, e.Team1.Name, e.Team2.Name, e.MatchDate, err)
		}
		resolved[key] = matches
	}
	return resolved, nil
}

// resolve settles the entry's state against the matches found for its fixture.
func (e *entry) resolve(resolved map[fixtureKey][]PlayedMatch, today time.Time) {
	if e.isScenario() {
		e.State = StateScenario
		return
	}
	if e.State == StateSuperseded {
		return
	}
	e.IssuedAfterMatchDate = !e.issuedOnOrBeforeMatchDate()
	matches := resolved[keyOf(e.fixture())]
	switch len(matches) {
	case 0:
		e.State = StateUnresolved
		days := daysBetween(e.stored.MatchDate, today)
		e.DaysPastMatchDate = &days
		return
	case 1:
		e.settle(matches[0])
	default:
		// Two matches between the same sides on the same day in one format is a
		// double-header the record cannot tell apart; it says so rather than picking.
		e.State = StateUnresolved
		days := daysBetween(e.stored.MatchDate, today)
		e.DaysPastMatchDate = &days
		e.StateNote = fmt.Sprintf(
			"%d matches between these sides on that date; the record cannot say which was meant",
			len(matches),
		)
	}
}

// settle is the scored / no-result decision against the one match found, and the score.
//
// A forecast issued after the day it was about is settled and scored exactly like any
// other -- the row shows what it claimed and what happened -- but it lands in
// StatePostHoc, which no summary is over: it was served from ratings that may already
// hold the result, so its Brier and coverage measure hindsight, not the model (GO-03).
func (e *entry) settle(match PlayedMatch) {
	e.Happened = happened(e, match)
	if match.WinnerOppositionID == nil {
		e.State = StateNoResult
		return
	}
	e.State = StateScored
	if e.IssuedAfterMatchDate {
		e.State = StatePostHoc
	}
	teamOneWon := *match.WinnerOppositionID == e.stored.Team1OppositionID
	e.p = e.stored.WinProbabilityTeam1
	if teamOneWon {
		e.y = 1
	}
	score := &Score{Team1Won: teamOneWon, Brier: (e.p - e.y) * (e.p - e.y)}
	if e.Claimed.Team1Range != nil && e.Happened.Team1Total != nil {
		hit := covered(*e.Happened.Team1Total, *e.Claimed.Team1Range)
		score.Team1Covered = &hit
	}
	if e.Claimed.Team2Range != nil && e.Happened.Team2Total != nil {
		hit := covered(*e.Happened.Team2Total, *e.Claimed.Team2Range)
		score.Team2Covered = &hit
	}
	if e.payload != nil {
		named1, named2 := playerIDs(e.payload.Team1), playerIDs(e.payload.Team2)
		score.ElevenOverlap = ElevenOverlap{
			Team1Matched: overlap(named1, e.stored.Team1OppositionID, match.FieldedPlayers),
			Team2Matched: overlap(named2, e.stored.Team2OppositionID, match.FieldedPlayers),
			Of:           len(named1) + len(named2),
		}
		score.ElevenOverlap.Matched = score.ElevenOverlap.Team1Matched + score.ElevenOverlap.Team2Matched
	}
	e.Score = score
}

// happened reads the match into the entry's orientation: which side is team1 here is the
// prediction's choice, not the match's.
func happened(e *entry, match PlayedMatch) *Happened {
	h := &Happened{
		MatchID:            match.MatchID,
		WinnerOppositionID: match.WinnerOppositionID,
		OutcomeByRuns:      match.OutcomeByRuns,
		OutcomeByWickets:   match.OutcomeByWickets,
	}
	for _, innings := range match.Innings {
		runs := innings.Runs
		switch innings.BattingOppositionID {
		case e.stored.Team1OppositionID:
			if h.Team1Total == nil {
				h.Team1Total = &runs
			}
		case e.stored.Team2OppositionID:
			if h.Team2Total == nil {
				h.Team2Total = &runs
			}
		}
	}
	if len(match.Innings) > 0 {
		first := match.Innings[0].BattingOppositionID == e.stored.Team1OppositionID
		h.Team1BattedFirst = &first
	}
	return h
}

// daysBetween is whole days from the match date to today, negative before the match.
func daysBetween(matchDate, today time.Time) int {
	day := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	return int(day.Sub(matchDate).Hours() / 24)
}

func countStates(entries []*entry) map[string]int {
	counts := make(map[string]int, len(States()))
	for _, state := range States() {
		counts[state] = 0
	}
	for _, e := range entries {
		counts[e.State]++
	}
	return counts
}

// scored is the rows every summary is over. It selects StateScored alone, which is what
// keeps a post-hoc forecast's score on its own row and out of the aggregates (GO-03).
func scored(entries []*entry) []*entry {
	out := make([]*entry, 0, len(entries))
	for _, e := range entries {
		if e.State == StateScored {
			out = append(out, e)
		}
	}
	return out
}

func summariseWins(entries []*entry) WinSummary {
	rows := scored(entries)
	type outcomes struct{ p, y []float64 }
	all := &outcomes{}
	byFormat := map[string]*outcomes{}
	for _, e := range rows {
		all.p, all.y = append(all.p, e.p), append(all.y, e.y)
		format := byFormat[e.Format]
		if format == nil {
			format = &outcomes{}
			byFormat[e.Format] = format
		}
		format.p, format.y = append(format.p, e.p), append(format.y, e.y)
	}
	summary := WinSummary{
		Overall:         winScore(all.p, all.y),
		ByFormat:        make(map[string]WinScore, len(byFormat)),
		Reliability:     reliability(all.p, all.y, ReliabilityBins),
		ReliabilityBins: ReliabilityBins,
	}
	for format, o := range byFormat {
		summary.ByFormat[format] = winScore(o.p, o.y)
	}
	return summary
}

// summariseCoverage keeps every (format, population) apart; the innings are labelled by
// the order they were played in, as the harness labels them.
func summariseCoverage(entries []*entry) CoverageSummary {
	type cell struct {
		n            int
		first, chase [2]int // n, covered
	}
	cells := map[[2]string]*cell{}
	populations := make(map[string]int, len(Populations()))
	for _, population := range Populations() {
		populations[population] = 0
	}
	for _, e := range scored(entries) {
		populations[e.Population]++
		key := [2]string{e.Format, e.Population}
		c := cells[key]
		if c == nil {
			c = &cell{}
			cells[key] = c
		}
		c.n++
		if e.Score == nil || e.Happened == nil || e.Happened.Team1BattedFirst == nil {
			continue
		}
		team1First := *e.Happened.Team1BattedFirst
		tally := func(hit *bool, first bool) {
			if hit == nil {
				return
			}
			slot := &c.chase
			if first {
				slot = &c.first
			}
			slot[0]++
			if *hit {
				slot[1]++
			}
		}
		tally(e.Score.Team1Covered, team1First)
		tally(e.Score.Team2Covered, !team1First)
	}
	rows := make([]CoverageRow, 0, len(cells))
	for key, c := range cells {
		rows = append(rows, CoverageRow{
			Format:       key[0],
			Population:   key[1],
			NPredictions: c.n,
			FirstInnings: coverageScore(c.first[0], c.first[1]),
			Chase:        coverageScore(c.chase[0], c.chase[1]),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Format != rows[j].Format {
			return rows[i].Format < rows[j].Format
		}
		return rows[i].Population < rows[j].Population
	})
	return CoverageSummary{Rows: rows, Populations: populations}
}

func summariseElevens(entries []*entry) ElevensSummary {
	summary := ElevensSummary{}
	overlaps := make([]float64, 0)
	for _, e := range scored(entries) {
		if e.Score == nil || e.Score.ElevenOverlap.Of == 0 {
			continue
		}
		matched := e.Score.ElevenOverlap.Matched
		overlaps = append(overlaps, float64(matched))
		if matched == e.Score.ElevenOverlap.Of {
			summary.Complete++
		}
		if summary.Min == nil || matched < *summary.Min {
			m := matched
			summary.Min = &m
		}
		if summary.Max == nil || matched > *summary.Max {
			m := matched
			summary.Max = &m
		}
	}
	summary.N = len(overlaps)
	summary.Mean = mean(overlaps)
	return summary
}
