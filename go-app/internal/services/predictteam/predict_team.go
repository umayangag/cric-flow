// Package predictteam answers "who should play, and what will happen" for a future match.
//
// Everything it returns comes from the XI layer in ml-service (plan L2/L3): the XI from
// /xi/optimize, the displayed win probability from /xi/predict-win, and the totals,
// per-player points and their ranges from /simulate — or, where there is no innings length,
// from /performance/predict. Nothing here scores players, weighs a batting score against a
// bowling one, or rescales one model's answer toward another's; those were the windowed-form
// path and they are gone (plan P-5).
//
// go-app's remaining job is the part ml-service cannot know: who is available (the pool),
// which fixture this is (format, teams, venue, date), and what the caller requires of an XI.
package predictteam

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// Input defines the request for future-match team selection.
//
// Team1 and Team2 are *references to a side*, not names: a club id, or a name with the
// gender that makes it one. A bare name is accepted only where the format holds exactly one
// side of that name; where it holds two, the request is refused rather than resolved by
// guess (D-10).
type Input struct {
	Format string     `json:"format"`
	Team1  db.TeamRef `json:"team1"`
	Team2  db.TeamRef `json:"team2"`
	Venue  string     `json:"venue,omitempty"` // venue name; empty = no venue named
	// MatchDate is the day the fixture is played. It is a calendar day, not an instant:
	// everything downstream reads a day from it -- the pool's cutoff, the as-of date, the
	// `match_date` ml-service is sent, the `date` the issued prediction is stored in --
	// and the day is taken from the value's own zone (GO-09).
	MatchDate     time.Time `json:"match_date"`
	ExtraTeam1    []int64   `json:"extra_team1,omitempty"` // extra player IDs for team1 (e.g. IPL auction)
	ExtraTeam2    []int64   `json:"extra_team2,omitempty"` // extra player IDs for team2
	MinBowlers    int       `json:"min_bowlers,omitempty"` // default from config
	RequireKeeper bool      `json:"require_keeper,omitempty"`
	// Team1BatsFirst is the toss, when it is known: true where team1 bats first, false
	// where team2 does. Nil is the default and means unknown, which is the marginalised
	// behaviour the simulator has always had — half the draws each way.
	Team1BatsFirst *bool `json:"team1_bats_first,omitempty"`
	// Team1XI and Team2XI pin each side's eleven by player id: Play mode (P1-2) scores
	// the eleven the caller built and never re-optimises underneath them. Both sides are
	// pinned or neither is — an answer that searched one side while the user was editing
	// the other would move numbers the user did not touch.
	Team1XI []int64 `json:"team1_xi,omitempty"`
	Team2XI []int64 `json:"team2_xi,omitempty"`
	// Team1Pool and Team2Pool are what the caller asked for about each side's
	// candidates: the recency window, whether to widen it to all-time, and a manual
	// pick. The zero value is the per-format recency default (D-12).
	Team1Pool PoolRequest `json:"team1_pool,omitempty"`
	Team2Pool PoolRequest `json:"team2_pool,omitempty"`
	// Actor is whose retirement ledger applies. Empty means the default user; the
	// ledger is skipped entirely on a backtest, whatever this says.
	Actor string `json:"actor,omitempty"`
	// AsOf, when set, asks for ratings as they stood strictly before this date instead of
	// "through today". Backtests over played matches set it to the match date, so a
	// prediction provably cannot see the match's own result or any later one; live
	// predictions leave it zero.
	AsOf time.Time `json:"as_of,omitempty"`
}

// Constraints are what the caller knows about an XI and the model does not: how many
// players, how many bowling options, whether a keeper is required. ml-service reads them
// off the same as-of vectors the objective reads, so "a bowling option" means one thing.
type Constraints struct {
	Size          int
	MinBowlers    int
	RequireKeeper bool
}

// ValueRange is a 10-90 range shown beside a point.
type ValueRange struct {
	P10 float64 `json:"p10"`
	P90 float64 `json:"p90"`
}

// SelectedPlayer is one player in the selected XI, with the point the scorecard shows, the
// 10-90 range around it, and the two L3 explanations: what the XI loses without this player
// and how much of the innings total's spread he accounts for.
type SelectedPlayer struct {
	PlayerID int64 `json:"player_id"`
	// PlayerKey is the registry id ml-service knows this player by. It is how the
	// simulator's and the optimiser's answers are matched back to these rows, and it stays
	// off the wire: a client joins on player_id, which is this repo's own identifier.
	PlayerKey         string      `json:"-"`
	PlayerName        string      `json:"player_name"`
	Runs              float64     `json:"runs"`
	RunsRange         *ValueRange `json:"runs_range,omitempty"`
	Balls             float64     `json:"balls,omitempty"`
	BallsRange        *ValueRange `json:"balls_range,omitempty"`
	Wickets           float64     `json:"wickets"`
	WicketsRange      *ValueRange `json:"wickets_range,omitempty"`
	RunsConceded      float64     `json:"runs_conceded"`
	RunsConcededRange *ValueRange `json:"runs_conceded_range,omitempty"`
	// Economy is runs conceded per over, and needs a balls-bowled forecast to exist: only
	// the simulator produces one, so it is zero on the performance-only path.
	Economy float64 `json:"economy,omitempty"`
	// MarginalValue is P(win) lost if this player were replaced by an average one. Absent
	// on a rating-ordered XI: nothing was maximised, so nothing has a margin.
	MarginalValue *float64 `json:"marginal_value,omitempty"`
	// SpreadShare is the player's share of the side total's variance, from the simulator.
	SpreadShare *float64 `json:"spread_share,omitempty"`
	// SelectionReason is what the selection read about this player — his role, his rating
	// standing in the pool he was picked out of, and the best alternative left in it
	// (P1-3, selection_reason.go). Absent in Play mode: the caller built the eleven, so
	// there is no selection to explain.
	SelectionReason *PlayerSelectionReason `json:"selection_reason,omitempty"`
}

// SelectionSummary says how the XIs were chosen.
//
// Optimised is false where the objective does not rank (H-17: TEST): the XI is the
// rating-ordered pick, which is a selection but not an optimised one, and every surface
// that shows it has to say so.
type SelectionSummary struct {
	Objective string `json:"objective"` // "win", "ratings" or "fixed"
	Optimised bool   `json:"optimised"`
	Note      string `json:"note,omitempty"`
	// MustInclude says what became of the must-include ids: present where any were asked
	// for on a selected (not pinned) eleven, naming the ones it does not hold (P1-4).
	MustInclude *MustIncludeReport `json:"must_include,omitempty"`
}

// ForecastSummary says which model produced the per-player numbers, and — where that is
// not the default — why.
//
// It exists because §8.7's rule applies to more than the win probability: a fallback that
// changes which model answered must say so in the response, not only in a log line. A
// format with no innings length gets L2-B's own quantiles instead of the simulator's
// draws, and before this the only sign was a `scorecard` that quietly was not there.
//
// Source is "simulator" (whole matches drawn, L2-C) or "performance_quantiles" (L2-B's
// per-player distributions, reported directly).
type ForecastSummary struct {
	Source string `json:"source"`
	Note   string `json:"note,omitempty"`
}

// WinProbabilitySummary is the headline probability and where it came from.
//
// Source is "display" (the monotone GBM over both elevens) or "simulator" (the share of
// simulated matches won). E2 decided which is the headline per format; the other is
// reported beside it, never blended.
type WinProbabilitySummary struct {
	Team1           float64  `json:"team1"`
	Source          string   `json:"source"`
	Simulated       *float64 `json:"simulated,omitempty"`
	PredictedWinner string   `json:"predicted_winner"`
}

// InningsTotal is one simulated innings: the median-band total the scorecard lines and
// extras sum to, and the distribution the draws actually produced.
type InningsTotal struct {
	Total  float64 `json:"total"`
	Extras float64 `json:"extras"`
	P10    float64 `json:"p10"`
	Median float64 `json:"median"`
	P90    float64 `json:"p90"`
}

// Scorecard is the simulated match: present only for formats with an innings length.
//
// The two innings are named by *side*, not by batting position: the simulator reports
// team1's innings and team2's innings whichever bats first. They were called `innings1` and
// `innings2`, which was invisible while the toss was always unknown and wrong the moment it
// could be named — "innings 1 (India)" beside "Australia bats first" (P1-1).
type Scorecard struct {
	Samples          int  `json:"samples"`
	TossMarginalised bool `json:"toss_marginalised"`
	// SharedFactor says whether the simulator that drew this scorecard carried a shared
	// match factor. A format whose calibration fold was too thin ships without one, and
	// its 10-90 ranges are a different population's (B-12); the track record keeps the
	// two apart, so the answer has to say which it was (P2-4).
	SharedFactor bool         `json:"shared_factor"`
	Team1Innings InningsTotal `json:"team1_innings"`
	Team2Innings InningsTotal `json:"team2_innings"`
}

// ResolvedSide is the side a prediction actually scored.
//
// It is on the wire for unambiguous names as well as ambiguous ones, because §8.7's rule is
// about the *answer*, not about the doubt: naming the side only when the request was unclear
// would make silence mean "we agreed", and silence is exactly what D-10 was.
type ResolvedSide struct {
	ClubID      int64  `json:"club_id"`
	Name        string `json:"name"`
	Gender      string `json:"gender"`
	DisplayName string `json:"display_name"`
}

func newResolvedSide(side db.TeamSide) ResolvedSide {
	return ResolvedSide{
		ClubID:      side.ClubID,
		Name:        side.Name,
		Gender:      side.Gender,
		DisplayName: side.Label(),
	}
}

// Result is one prediction: two XIs, how they were chosen, the headline probability, and —
// where the format has an innings length — the simulated scorecard the player points and
// ranges come from.
//
// Every substitution it makes is named on the wire: `team1_side` and `team2_side` say which
// sides were scored (D-10), `selection` says whether the XIs were optimised or rating-ordered
// (H-17), `forecast` says which model produced the per-player numbers, `toss` says which
// batting order they were produced under, and `win_probability.source` says which model
// produced the headline probability. Nothing here falls back silently (§8.7).
//
// `run_id` and `ratings_through` (the embedded ServedRatings) say which rating state every
// number was computed from; they are the same on every ml-service call the prediction made,
// or the prediction is refused (P1-5).
type Result struct {
	ServedRatings
	Team1Side      ResolvedSide          `json:"team1_side"`
	Team2Side      ResolvedSide          `json:"team2_side"`
	Team1          []SelectedPlayer      `json:"team1"`
	Team2          []SelectedPlayer      `json:"team2"`
	Selection      SelectionSummary      `json:"selection"`
	Forecast       ForecastSummary       `json:"forecast"`
	WinProbability WinProbabilitySummary `json:"win_probability"`
	// Toss says which batting order the numbers assume, and whether a named one was used.
	Toss TossSummary `json:"toss"`
	// Venue says which venue every model read, or that none was named. A venue the caller
	// named and this database does not hold is refused rather than reported here (GO-08).
	Venue     VenueSummary `json:"venue"`
	Scorecard *Scorecard   `json:"scorecard,omitempty"`
	// Team1PoolSummary and Team2PoolSummary say which candidates each XI was chosen out
	// of: the window, the size, and every player the ledger excluded (D-12).
	Team1PoolSummary PoolSummary `json:"team1_pool"`
	Team2PoolSummary PoolSummary `json:"team2_pool"`
	// Constraints says whether each pinned eleven meets what was asked of it. Present
	// only in Play mode: an eleven the optimiser chose was chosen under the constraints,
	// while one the caller built is checked against them and never repaired (P1-2).
	Constraints *ConstraintReport `json:"constraints,omitempty"`
}

// CrossGenderFixtureError reports a fixture whose two sides are not the same gender.
//
// It is refused rather than predicted. The two sides would be scored against rating state
// and context baselines that describe different games, and the model has never seen such a
// match, so the number it returned would be arithmetic without a referent.
type CrossGenderFixtureError struct {
	Team1 db.TeamSide
	Team2 db.TeamSide
}

func (e *CrossGenderFixtureError) Error() string {
	return fmt.Sprintf("%s and %s are not the same gender; no such fixture is played",
		e.Team1.Label(), e.Team2.Label())
}

// XIService is everything the prediction path needs from ml-service.
//
// One interface because after P-5 there is one path: the selection, the displayed
// probability, the simulated match and the per-player forecasts are all functions of the
// same as-of rating state, keyed by player id. Nothing here takes a feature map.
type XIService interface {
	OptimizeXI(ctx context.Context, req XIOptimizationRequest) (*XIOptimizationResult, error)
	PredictMatchWinXI(ctx context.Context, req XIWinRequest) (*XIWinResult, error)
	SimulateMatchXI(ctx context.Context, req XISimulationRequest) (*XISimulationResult, error)
	PredictPerformance(ctx context.Context, req XIPerformanceRequest) (*XIPerformanceResult, error)
}

// fixture is the resolved match: the two sides, and ids for everything the ML service is
// told about.
type fixture struct {
	format       string
	team1, team2 db.TeamSide
	// venue is the venue the answer was produced at, resolved once here and reported on
	// the wire so an unresolved one is never mistaken for a fixture nobody named a ground
	// for (GO-08).
	venue        VenueSummary
	pool1, pool2 []db.PlayerPoolRow
	summary1     PoolSummary
	summary2     PoolSummary
	constraints  Constraints
	// mustInclude1 and mustInclude2 are each side's must-include ids as registry ids: the
	// lock the search is sent, and in Play mode the list the caller's eleven is checked
	// against. Resolved once, here, so the two paths cannot disagree about who was asked
	// for.
	mustInclude1 []string
	mustInclude2 []string
	asOf         time.Time
	// matchDate is the day the fixture is played, which is the date every date-dependent
	// feature is read at (SERVE-04). It is not asOf, which chooses *which ratings* answer
	// and is zero for a live request; ml-service used to have neither and dated every
	// fixture by the last match in its own state, which the archive leaves days behind.
	matchDate time.Time
	// gender is the fixture's, and both sides carry it: a cross-gender fixture is refused
	// above, so there is one gender to name. It picks the context baseline ml-service reads
	// the fixture's scoring rates from (SERVE-04).
	gender string
	// team1BatsFirst is the toss as the caller gave it; nil is unknown.
	team1BatsFirst *bool
	// pinned holds both elevens where the caller built them (Play mode); isPinned says
	// whether they did.
	pinned   pinnedXI
	isPinned bool
}

// PredictTeams picks both XIs and predicts the match.
func PredictTeams(ctx context.Context, input Input, service XIService) (*Result, error) {
	fix, err := resolveFixture(ctx, input)
	if err != nil {
		return nil, err
	}

	selection, err := chooseXIs(ctx, service, fix)
	if err != nil {
		return nil, err
	}
	if err := refuseSharedSelection(selection.Team1Keys, selection.Team2Keys); err != nil {
		return nil, err
	}
	selection.Summary.MustInclude = mustIncludeReport(input, fix, selection)

	result := &Result{
		ServedRatings:    selection.Served,
		Team1Side:        newResolvedSide(fix.team1),
		Team2Side:        newResolvedSide(fix.team2),
		Team1:            newSelectedPlayers(selection.Team1Keys, fix.pool1, selection.Team1Answers),
		Team2:            newSelectedPlayers(selection.Team2Keys, fix.pool2, selection.Team2Answers),
		Selection:        selection.Summary,
		Venue:            fix.venue,
		Team1PoolSummary: fix.summary1,
		Team2PoolSummary: fix.summary2,
	}

	win, err := service.PredictMatchWinXI(ctx, newWinRequest(fix, selection))
	if err != nil {
		return nil, fmt.Errorf("win probability: %w", err)
	}
	if err := refuseTossMismatch(
		"win probability", fix.team1BatsFirst, win.TossMarginalised, "toss_marginalised"); err != nil {
		return nil, err
	}
	if err := result.Adopt(win.Served); err != nil {
		return nil, fmt.Errorf("win probability: %w", err)
	}
	if fix.isPinned {
		if win.Team1Check == nil || win.Team2Check == nil {
			return nil, fmt.Errorf(
				"win probability: the constraints were sent for checking and the answer carried no check")
		}
		result.Constraints = newConstraintReport(fix, win.Team1Check, win.Team2Check)
	}
	result.WinProbability = WinProbabilitySummary{
		Team1:           win.Team1WinProbability,
		Source:          winProbabilitySourceDisplay,
		PredictedWinner: winnerFrom(win.Team1WinProbability, fix.team1, fix.team2),
	}

	if err := applyMatchForecast(ctx, service, fix, selection.Team1Keys, selection.Team2Keys, result); err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "prediction served",
		slog.String("format", fix.format),
		slog.String("run_id", result.RunID),
		slog.String("ratings_through", result.RatingsThrough))
	return result, nil
}

// newWinRequest is the fixture and the two chosen elevens as /xi/predict-win takes them.
//
// The toss goes with them. The display model reads the batting order in every format — it
// is the innings a side's aggregates are read for, not a property of the simulator — so a
// caller who names one is answered at that order rather than over the average of both,
// which was the defect: the field existed on both sides of the wire and go-app never set
// it (GO-07).
func newWinRequest(fix fixture, selection xiSelection) XIWinRequest {
	req := XIWinRequest{
		Format:          fix.format,
		Team1PlayerKeys: selection.Team1Keys,
		Team2PlayerKeys: selection.Team2Keys,
		Team1ID:         fix.team1.ClubID,
		Team2ID:         fix.team2.ClubID,
		VenueID:         fix.venue.VenueID,
		Team1BatsFirst:  fix.team1BatsFirst,
		AsOf:            fix.asOf,
	}
	// A pinned eleven is checked against the constraints on the call that scores it, so
	// the check describes the same eleven the probability beside it describes.
	if fix.isPinned {
		req.Team1Constraints, req.Team2Constraints = constraintCheckRequests(fix)
	}
	return req
}

// applyMatchForecast fills in the per-player points and ranges, and the scorecard where
// there is one to fill: the simulator for formats with an innings length, the performance
// model's own quantiles for the rest (they have no innings to draw, but every player still
// has a forecast).
func applyMatchForecast(
	ctx context.Context,
	service XIService,
	fix fixture,
	xi1, xi2 []string,
	result *Result,
) error {
	if FormatHasInningsLength(fix.format) {
		return applyXISimulation(ctx, service, fix, xi1, xi2, result)
	}
	return applyPerformanceForecast(ctx, service, fix, xi1, xi2, result)
}

// resolveFixture turns the two side references into the sides ml-service is addressed with,
// and loads both player pools. Availability is the caller's knowledge, not the model's
// (plan §9.4).
func resolveFixture(ctx context.Context, input Input) (fixture, error) {
	format := NormalizeFormat(input.Format)
	if format == "" || input.Team1.IsEmpty() || input.Team2.IsEmpty() {
		err := fmt.Errorf("format, team1, team2 are required")
		slog.Error("predictteam.PredictTeams validation failed", slog.Any("err", err))
		return fixture{}, err
	}
	// The pool's cutoff is the fixture's own calendar day, derived where the query is
	// built so that both sides and the candidate list agree on it (GO-09).
	cutoff := input.MatchDate

	team1, err := resolveSide(ctx, input.Team1, format, "team1")
	if err != nil {
		return fixture{}, err
	}
	team2, err := resolveSide(ctx, input.Team2, format, "team2")
	if err != nil {
		return fixture{}, err
	}
	if team1.Gender != team2.Gender {
		err := &CrossGenderFixtureError{Team1: team1, Team2: team2}
		slog.Warn("predictteam.PredictTeams refused a cross-gender fixture",
			slog.String("team1", team1.Label()), slog.String("team2", team2.Label()))
		return fixture{}, err
	}

	venue, err := resolveVenue(ctx, input.Venue, productionVenueLookup)
	if err != nil {
		return fixture{}, err
	}

	constraints := resolveConstraints(input)

	// The ledger applies to upcoming-match requests only. A backtest names its as-of
	// date, and a retirement flagged today says nothing about who was available then.
	applyLedger := input.AsOf.IsZero()
	var flags map[int64]availability.Flag
	if applyLedger {
		flags, err = poolFlags(ctx, input.Actor)
		if err != nil {
			return fixture{}, err
		}
	}

	// A pinned player joins his side's candidates whatever the window or the ledger says,
	// the way a must-include id does: the caller has named him as playing, which is
	// better evidence about availability than either (P1-2).
	pool1, summary1, err := loadPool(
		ctx, format, team1, cutoff, input.Team1Pool,
		append(append([]int64{}, input.ExtraTeam1...), input.Team1XI...), flags, applyLedger, constraints.Size)
	if err != nil {
		return fixture{}, err
	}
	pool2, summary2, err := loadPool(
		ctx, format, team2, cutoff, input.Team2Pool,
		append(append([]int64{}, input.ExtraTeam2...), input.Team2XI...), flags, applyLedger, constraints.Size)
	if err != nil {
		return fixture{}, err
	}

	// Both pools were loaded from one player table by club, so a player who moved clubs
	// inside the window is in both of them. He leaves one of them here, before anything
	// resolves a must-include id or scores an eleven (GO-04, shared_players.go).
	side1 := newCandidateSide(team1, pool1, &summary1, input.ExtraTeam1, input.Team1XI)
	side2 := newCandidateSide(team2, pool2, &summary2, input.ExtraTeam2, input.Team2XI)
	if err := resolveSharedCandidates(&side1, &side2); err != nil {
		return fixture{}, err
	}
	pool1, pool2 = side1.rows, side2.rows
	if err := checkPoolSize(team1.Label(), len(pool1), constraints.Size, summary1); err != nil {
		return fixture{}, err
	}
	if err := checkPoolSize(team2.Label(), len(pool2), constraints.Size, summary2); err != nil {
		return fixture{}, err
	}

	mustInclude1, err := mustIncludeKeys(team1, pool1, input.ExtraTeam1)
	if err != nil {
		return fixture{}, err
	}
	mustInclude2, err := mustIncludeKeys(team2, pool2, input.ExtraTeam2)
	if err != nil {
		return fixture{}, err
	}

	fix := fixture{
		format:         format,
		team1:          team1,
		team2:          team2,
		venue:          venue,
		pool1:          pool1,
		pool2:          pool2,
		summary1:       summary1,
		summary2:       summary2,
		constraints:    constraints,
		mustInclude1:   mustInclude1,
		mustInclude2:   mustInclude2,
		asOf:           input.AsOf,
		matchDate:      input.MatchDate,
		gender:         servedGender(team1.Gender),
		team1BatsFirst: input.Team1BatsFirst,
	}
	if err := applyPinnedXIs(&fix, input); err != nil {
		return fixture{}, err
	}
	return fix, nil
}

// servedGender is the fixture gender ml-service will accept, or empty.
//
// ml-service names the two context groups the rating pass builds, and anything else is a
// value it would refuse the whole request over. A row whose gender the archive never
// recorded is therefore sent as no gender at all, which reads the unsplit baseline — the
// behaviour every request had before SERVE-04 — rather than turning a prediction into a
// 422 over a field that only matters where the gender split is on.
func servedGender(gender string) string {
	switch gender {
	case "male", "female":
		return gender
	default:
		return ""
	}
}

// applyPinnedXIs resolves both pinned elevens, where the caller sent them.
//
// One side pinned and the other not is refused rather than half-honoured: the answer
// would search an eleven the user is not looking at while pinning the one they are, and
// the two numbers on screen would have been produced by two different questions.
func applyPinnedXIs(fix *fixture, input Input) error {
	if len(input.Team1XI) == 0 && len(input.Team2XI) == 0 {
		return nil
	}
	if len(input.Team1XI) == 0 || len(input.Team2XI) == 0 {
		err := fmt.Errorf("both elevens are pinned or neither is: team1_xi has %d players and team2_xi has %d",
			len(input.Team1XI), len(input.Team2XI))
		slog.Warn("predictteam.PredictTeams refused a half-pinned fixture", slog.Any("err", err))
		return err
	}
	keys1, err := resolvePinnedXI(fix.team1, fix.pool1, input.Team1XI, fix.constraints.Size)
	if err != nil {
		return err
	}
	keys2, err := resolvePinnedXI(fix.team2, fix.pool2, input.Team2XI, fix.constraints.Size)
	if err != nil {
		return err
	}
	fix.pinned = pinnedXI{
		team1Keys:    keys1,
		team2Keys:    keys2,
		mustInclude1: fix.mustInclude1,
		mustInclude2: fix.mustInclude2,
	}
	fix.isPinned = true
	return nil
}

// resolveSide resolves one side reference, keeping the resolver's own error intact so the
// handler can turn an ambiguous name into a 400 that names both candidates.
func resolveSide(ctx context.Context, ref db.TeamRef, format, field string) (db.TeamSide, error) {
	side, err := db.ResolveTeamSide(ctx, ref, format)
	if err != nil {
		slog.Error("predictteam.PredictTeams resolve side failed",
			slog.String("field", field),
			slog.Int64("club_id", ref.ClubID),
			slog.String("name", ref.Name),
			slog.String("gender", ref.Gender),
			slog.Any("err", err))
		return db.TeamSide{}, fmt.Errorf("resolve %s: %w", field, err)
	}
	return side, nil
}

func resolveConstraints(input Input) Constraints {
	cfg := config.Load()
	// The size is a constant, not a setting: ml-service refuses any side that is not an
	// eleven, because every model behind it was fitted on elevens (SERVE-02).
	size := config.DefaultTeamSize
	minBowlers := input.MinBowlers
	if minBowlers <= 0 {
		minBowlers = config.DefaultMinBowlers
		if cfg != nil && cfg.Team.MinBowlers > 0 {
			minBowlers = cfg.Team.MinBowlers
		}
	}
	return Constraints{Size: size, MinBowlers: minBowlers, RequireKeeper: input.RequireKeeper}
}

// newSelectedPlayers turns the chosen registry keys into response rows, in the order the
// optimiser returned them, resolving each back to the pool row he was chosen out of.
//
// A key the pool cannot resolve is dropped rather than returned as a nameless row with a
// zero id: it would mean ml-service answered with a player nobody asked about.
func newSelectedPlayers(
	keys []string,
	pool []db.PlayerPoolRow,
	answers sideAnswers,
) []SelectedPlayer {
	byKey := make(map[string]db.PlayerPoolRow, len(pool))
	for _, p := range pool {
		byKey[p.ExternalID] = p
	}
	out := make([]SelectedPlayer, 0, len(keys))
	for _, key := range keys {
		row, ok := byKey[key]
		if !ok {
			slog.Warn("selection returned a player the pool does not hold", slog.String("player_key", key))
			continue
		}
		player := SelectedPlayer{PlayerID: row.PlayerID, PlayerKey: key, PlayerName: row.PlayerName}
		if v, ok := answers.Marginals[key]; ok {
			value := v
			player.MarginalValue = &value
		}
		if reason, ok := answers.Reasons[key]; ok {
			resolved := newSelectionReason(reason, byKey)
			player.SelectionReason = &resolved
		}
		out = append(out, player)
	}
	return out
}

// winnerFrom names the side the headline probability favours. Exactly 0.5 is team2's, as
// it has always been; the probability is displayed beside it, so nothing is hidden.
//
// It names the *resolved* side -- "India (women)", not the "India" a caller typed -- for the
// same reason the response echoes both sides: the answer says what was scored.
func winnerFrom(team1Probability float64, team1, team2 db.TeamSide) string {
	if team1Probability >= 0.5 {
		return team1.Label()
	}
	return team2.Label()
}

// poolPlayerKeys is the pool as ml-service addresses it: registry ids, skipping anyone the
// importer never matched to a registry entry (0 rows today) because ml-service has no
// rating for a player it has never been told about under that name.
func poolPlayerKeys(pool []db.PlayerPoolRow) []string {
	keys := make([]string, 0, len(pool))
	seen := make(map[string]bool, len(pool))
	for _, p := range pool {
		if p.ExternalID == "" {
			slog.Warn("player has no registry id and cannot be selected",
				slog.Int64("player_id", p.PlayerID), slog.String("player_name", p.PlayerName))
			continue
		}
		// Two player rows can carry one registry id (two imports of one person). To the
		// selection that is one candidate offered twice, and a pool that offers him twice
		// can field him twice, so he is offered once (SERVE-02).
		if seen[p.ExternalID] {
			slog.Warn("two candidates share one registry id; the pool offers him once",
				slog.Int64("player_id", p.PlayerID), slog.String("player_name", p.PlayerName))
			continue
		}
		seen[p.ExternalID] = true
		keys = append(keys, p.ExternalID)
	}
	return keys
}
