package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// The projection (P3-2): what a candidate is projected to produce in a named eleven,
// against a named opposition, at named grounds — as the stack serves it.
//
// **The rule that is the item.** Valuation and projection, never XI-picking. Nothing here
// calls `/xi/optimize`, nothing here computes a marginal value, and the type this maps
// `/simulate` into carries no win probability at all (`ml_auction_client.go`), so a P(win)
// cannot reach the surface by accident. The record is that optimised selection in domestic
// T20 is indistinguishable from rating order (plan §8.8) and the IPL is domestic T20; a
// probability of winning beside a purchase would be that claim in another coat.
//
// **Everything is as served.** Two intervals appear on one row and they are different
// populations: L2-B's quantile heads, at nominal coverage on the harness (plan §8.2), and
// the simulator's drawn totals, which are B-11's open defect. Each names its source. None
// is widened, narrowed, adjusted for day or night, or hidden.
//
// **The candidate's forecast is not batched, and cannot be.** `ml.xi.rows.player_feature_rows`
// builds each player's `own_*` columns from `aggregate_side` over the *whole* side it is
// given, so listing several candidates on one side would change every row's aggregates and
// project each man into a side of twelve or twenty rather than the eleven he would join.
// `/simulate` is stronger still: the innings it draws is the side it is handed. So the cost
// is one `/performance/predict` and one `/simulate` per candidate per ground, and P3-2
// measures it rather than assuming it away.

// auctionProjectionRequest is one candidate, one toss assumption and an optional venue mix.
type auctionProjectionRequest struct {
	PlayerID int64 `json:"player_id"`
	// Team1BatsFirst is the toss. Absent means unknown, and the model marginalises over
	// both batting orders — which is the honest default months before a fixture exists.
	Team1BatsFirst *bool `json:"team1_bats_first"`
	// VenueWeights is the operator's venue mix. Absent means no mixed range is computed:
	// a mix is a claim about how often the eleven plays where, and nothing in this system
	// knows that.
	VenueWeights []auctionVenueWeight `json:"venue_weights"`
}

type auctionVenueWeight struct {
	VenueID int64   `json:"venue_id"`
	Weight  float64 `json:"weight"`
}

// auctionProjectionResponse is the answer: the assumptions it was made under, one row per
// ground, and the mixture where the operator asked for one.
//
// There is no `win_probability` field and no `marginal_value` field, on this type or on
// anything it holds. A test asserts the rendered payload holds no key containing "win".
type auctionProjectionResponse struct {
	AuctionID   string                    `json:"auction_id"`
	Candidate   auctionProjectionPlayer   `json:"candidate"`
	Assumptions auctionAssumptionsBlock   `json:"assumptions"`
	Grounds     []auctionGroundProjection `json:"grounds"`
	// Mixture is the eleven's total over the operator's venue mix, absent where no
	// weights were sent.
	Mixture *auctionMixture `json:"mixture,omitempty"`
	// Intervals names every interval source on this answer, once, with what each is and
	// what is open against it. The rows carry the source name; this is where a reader
	// learns what the name means without leaving the answer.
	Intervals     []auctionIntervalSource   `json:"intervals"`
	ServedRatings predictteam.ServedRatings `json:"served_ratings"`
}

// auctionProjectionPlayer is one named player on the answer.
type auctionProjectionPlayer struct {
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
}

// auctionAssumptionsBlock is the three things the operator named, carried back on every
// answer so a projection on screen says which guess it was made for.
type auctionAssumptionsBlock struct {
	// Eleven is the side the candidate was projected in — the likely eleven with him in
	// it, which is what was actually sent to the model.
	Eleven     []auctionProjectionPlayer `json:"eleven"`
	Opposition auctionOppositionBlock    `json:"opposition"`
	Grounds    []auctionGroundRef        `json:"grounds"`
	Toss       string                    `json:"toss"`
	Format     string                    `json:"format"`
	// WhatAGroundChanges is the sentence §6 requires beside the venue mix, read from the
	// code rather than asserted: it says what a ground does and does not move.
	WhatAGroundChanges string `json:"what_a_ground_changes"`
	// NotXIPicking is the record's own sentence, kept beside the numbers.
	NotXIPicking string `json:"not_xi_picking"`
}

type auctionOppositionBlock struct {
	ClubID  int64                     `json:"club_id"`
	Name    string                    `json:"name"`
	Players []auctionProjectionPlayer `json:"players"`
}

type auctionGroundRef struct {
	VenueID   int64  `json:"venue_id"`
	VenueName string `json:"venue_name"`
}

// The toss vocabulary of a projection, in the buyer's terms: the candidate's eleven is
// always team1 on the wire, so "team1 bats first" is spelled as what it means to him.
const (
	auctionTossUnknown  = "unknown"
	auctionTossBatFirst = "candidate_eleven_bats_first"
	auctionTossChasing  = "candidate_eleven_chases"
)

// auctionGroundProjection is one ground's row: the candidate's own forecast and the
// eleven's total, each with the source of its interval named.
type auctionGroundProjection struct {
	VenueID   int64  `json:"venue_id"`
	VenueName string `json:"venue_name"`
	// Ground is what the served state knows about it, and whether it therefore reads
	// neutral (§8.7).
	Ground auctionGroundContext `json:"ground"`
	// Candidate is L2-B's forecast for him in this eleven at this ground.
	Candidate auctionCandidateForecast `json:"candidate"`
	// ElevenTotal is the simulator's total for the eleven with him in it.
	ElevenTotal auctionElevenTotal `json:"eleven_total"`
	// TossMarginalised is true where the draws averaged both batting orders.
	TossMarginalised bool `json:"toss_marginalised"`
}

// auctionGroundContext is the ground as the model read it — and only as the model read it.
type auctionGroundContext struct {
	BatFirstRate float64 `json:"bat_first_rate"`
	Matches      float64 `json:"matches"`
	// Neutral is true where the served state has no matches at this ground: the rows read
	// at the prior, and the answer says so rather than presenting a substitution nobody
	// can see.
	Neutral bool   `json:"neutral"`
	Note    string `json:"note"`
}

// auctionQuantiles is one quantity's 10-50-90 with the source of the interval named.
type auctionQuantiles struct {
	Q10    float64 `json:"q10"`
	Median float64 `json:"median"`
	Q90    float64 `json:"q90"`
	// IntervalSource is one of auction.IntervalSources() (H-24).
	IntervalSource string `json:"interval_source"`
}

// auctionWicketDistribution is the wicket count: an expectation and three probabilities,
// with no interval. There are no quantile heads for wickets on this path and an interval
// derived from the probabilities would be one the model never produced — so the field
// naming an interval source is absent, deliberately.
type auctionWicketDistribution struct {
	Expected float64 `json:"expected"`
	P0       float64 `json:"p0"`
	P1       float64 `json:"p1"`
	P2Plus   float64 `json:"p2_plus"`
	Note     string  `json:"note"`
}

type auctionCandidateForecast struct {
	Runs         auctionQuantiles          `json:"runs"`
	BallsFaced   auctionQuantiles          `json:"balls_faced"`
	RunsConceded auctionQuantiles          `json:"runs_conceded"`
	Wickets      auctionWicketDistribution `json:"wickets"`
	// InningsMarginalised is true where the toss was unknown and both batting orders were
	// averaged for these quantiles.
	InningsMarginalised bool `json:"innings_marginalised"`
}

type auctionElevenTotal struct {
	Total auctionQuantiles `json:"total"`
	// SpreadShare is the candidate's share of this total's spread, over the same draws.
	SpreadShare float64 `json:"spread_share"`
	Samples     int     `json:"samples"`
	// SharedFactor says whether the simulator that drew this carried a shared match factor
	// (B-12).
	SharedFactor bool `json:"shared_factor"`
}

// auctionMixture is the eleven's total over the venue mix, inverted from the pooled draws.
type auctionMixture struct {
	Total   auctionQuantiles     `json:"total"`
	Weights []auctionVenueWeight `json:"weights"`
	Note    string               `json:"note"`
}

// auctionIntervalSource explains one of the two intervals on this answer.
//
// It carries no L-1 metric key. The surface spells the two keys itself, as literals
// asserted against the contract's `auction_metric_keys` (H-24), so a key the UI could not
// spell is a failing test rather than an interval rendered with no explainer behind it —
// and sending the key here as well would be the same mapping written twice.
type auctionIntervalSource struct {
	Source string `json:"source"`
	Label  string `json:"label"`
	// Caveats are the open defects a reader must be told about before reading the
	// interval — B-11 and B-14 for the simulator's, none for L2-B's.
	Caveats []string `json:"caveats,omitempty"`
	Note    string   `json:"note"`
}

// The prose the answer carries. It is on the wire and not only in the UI because a
// projection copied out of the product loses everything but itself, and what a ground does
// not change is the part a reader would otherwise supply for themselves.
const (
	whatAGroundChangesNote = "The performance model reads a ground through two columns only — its bat-first " +
		"rate and how many matches the served state has there (venue_bf_rate, venue_n). The ground's scoring " +
		"level was gated and recorded as a null (A-1: population mix, not venue), and is not consumed: " +
		"FIXTURE_CONTEXT_FAMILIES_KEPT is empty. So these rows differ by what the toss does at each ground, " +
		"and by nothing else about the ground."

	notXIPickingNote = "In T20 the system has not shown it can choose an eleven better than rating order " +
		"(plan §8.8), and this module does not try to: it projects output. There is no win probability and " +
		"no marginal value here, and no request from this endpoint reaches the optimiser."

	neutralGroundNote = "The served rating state has no matches at this ground, so it read at the prior " +
		"(bat-first rate 0.5) and contributed nothing: this row is the neutral one, said so rather than " +
		"presented as a projection at a ground the model knows."

	wicketsHaveNoIntervalNote = "Wickets are a count distribution, not a quantile head: L2-B produces no " +
		"q10 or q90 for them on this path, so none is shown and none is derived from these probabilities."

	mixtureNote = "The mixture over the named grounds, inverted from the simulator's draws pooled with the " +
		"weights above. It is not the average of the per-ground ranges: a mixture's quantiles are not the " +
		"mean of its parts' quantiles."
)

// auctionLookups is what the projection reads from the database beside the record: the
// grounds' names, and a side's last recorded eleven for the operator to start from.
//
// An interface so the handlers can be exercised against a scripted ml-service with no
// database, the way the rest of the auction handlers are.
type auctionLookups interface {
	VenueName(ctx context.Context, venueID int64) (string, error)
	LastFieldedEleven(ctx context.Context, formatCode string, oppositionID int64) (*db.LastFieldedEleven, error)
}

// databaseLookups is the process default.
type databaseLookups struct{}

func (databaseLookups) VenueName(ctx context.Context, venueID int64) (string, error) {
	return db.VenueName(ctx, venueID)
}

func (databaseLookups) LastFieldedEleven(
	ctx context.Context,
	formatCode string,
	oppositionID int64,
) (*db.LastFieldedEleven, error) {
	return db.ReadLastFieldedEleven(ctx, formatCode, oppositionID)
}

func (a *App) auctionReads() auctionLookups {
	if a != nil && a.auctionLookups != nil {
		return a.auctionLookups
	}
	return databaseLookups{}
}

// projectAuctionCandidate answers one candidate's projection, per ground.
func (a *App) projectAuctionCandidate(
	ctx context.Context,
	record *auction.Auction,
	request auctionProjectionRequest,
) (*auctionProjectionResponse, error) {
	candidate, err := candidateFromList(record, request.PlayerID)
	if err != nil {
		return nil, err
	}
	eleven, opposition, err := projectionSides(record, candidate)
	if err != nil {
		return nil, err
	}
	elevenKeys, err := auction.RegistryKeys(eleven, "the likely eleven")
	if err != nil {
		return nil, err
	}
	oppositionKeys, err := auction.RegistryKeys(opposition.Players, "the opposition")
	if err != nil {
		return nil, err
	}

	response := &auctionProjectionResponse{
		AuctionID: record.ID,
		Candidate: namedPlayerOnWire(candidate),
		Assumptions: auctionAssumptionsBlock{
			Eleven: namedPlayersOnWire(eleven),
			Opposition: auctionOppositionBlock{
				ClubID:  opposition.OppositionID,
				Name:    opposition.Name,
				Players: namedPlayersOnWire(opposition.Players),
			},
			Toss:               tossName(request.Team1BatsFirst),
			Format:             record.FormatCode,
			WhatAGroundChanges: whatAGroundChangesNote,
			NotXIPicking:       notXIPickingNote,
		},
		Grounds:   make([]auctionGroundProjection, 0, len(record.VenueIDs)),
		Intervals: intervalSourcesOnWire(),
	}

	drawsByVenue := make(map[int64][]float64, len(record.VenueIDs))
	for _, venueID := range record.VenueIDs {
		venueName, err := a.auctionReads().VenueName(ctx, venueID)
		if err != nil {
			return nil, err
		}
		ground, draws, err := a.projectAtGround(ctx, groundProjectionInputs{
			record:         record,
			candidate:      candidate,
			opposition:     opposition,
			elevenKeys:     elevenKeys,
			oppositionKeys: oppositionKeys,
			venueID:        venueID,
			venueName:      venueName,
			team1BatsFirst: request.Team1BatsFirst,
		}, &response.ServedRatings)
		if err != nil {
			return nil, err
		}
		response.Assumptions.Grounds = append(response.Assumptions.Grounds,
			auctionGroundRef{VenueID: venueID, VenueName: venueName})
		response.Grounds = append(response.Grounds, *ground)
		drawsByVenue[venueID] = draws
	}

	mixture, err := mixtureOverGrounds(request.VenueWeights, drawsByVenue)
	if err != nil {
		return nil, err
	}
	response.Mixture = mixture

	slog.InfoContext(ctx, "auction projection served",
		slog.String("auction_id", record.ID),
		slog.Int64("player_id", candidate.PlayerID),
		slog.Int("grounds", len(response.Grounds)),
		slog.String("toss", response.Assumptions.Toss),
		slog.String("run_id", response.ServedRatings.RunID))
	return response, nil
}

// groundProjectionInputs is one ground's worth of the projection's inputs, gathered so the
// per-ground call takes one argument rather than nine.
type groundProjectionInputs struct {
	record         *auction.Auction
	candidate      auction.NamedPlayer
	opposition     *auction.Opposition
	elevenKeys     []string
	oppositionKeys []string
	venueID        int64
	venueName      string
	team1BatsFirst *bool
}

// projectAtGround makes the two calls one ground needs and returns the row plus the draws
// behind its total.
func (a *App) projectAtGround(
	ctx context.Context,
	in groundProjectionInputs,
	served *predictteam.ServedRatings,
) (*auctionGroundProjection, []float64, error) {
	forecast, err := a.mlClient.PredictPerformance(ctx, predictteam.XIPerformanceRequest{
		Format:          in.record.FormatCode,
		Team1PlayerKeys: in.elevenKeys,
		Team2PlayerKeys: in.oppositionKeys,
		Team1ID:         in.record.BuyerOppositionID,
		Team2ID:         in.opposition.OppositionID,
		VenueID:         in.venueID,
		Team1BatsFirst:  in.team1BatsFirst,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := served.Adopt(forecast.Served); err != nil {
		return nil, nil, err
	}
	candidateForecast, err := forecastFor(forecast, in.candidate)
	if err != nil {
		return nil, nil, err
	}

	simulation, err := a.mlClient.SimulateForAuction(ctx, AuctionSimulationRequest{
		Format:          in.record.FormatCode,
		Team1PlayerKeys: in.elevenKeys,
		Team2PlayerKeys: in.oppositionKeys,
		Team1ID:         in.record.BuyerOppositionID,
		Team2ID:         in.opposition.OppositionID,
		VenueID:         in.venueID,
		Team1BatsFirst:  in.team1BatsFirst,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := served.Adopt(simulation.Served); err != nil {
		return nil, nil, err
	}
	share, ok := simulation.SpreadShareByKey[in.candidate.ExternalID]
	if !ok {
		return nil, nil, fmt.Errorf(
			"the simulator returned no line for the candidate %q, so his share of the total's spread is unknown",
			in.candidate.ExternalID)
	}

	ground := auctionGroundContext{
		BatFirstRate: forecast.VenueBatFirstRate,
		Matches:      forecast.VenueMatches,
		Neutral:      forecast.VenueNeutral,
	}
	if forecast.VenueNeutral {
		ground.Note = neutralGroundNote
	}
	return &auctionGroundProjection{
		VenueID:          in.venueID,
		VenueName:        in.venueName,
		Ground:           ground,
		Candidate:        *candidateForecast,
		TossMarginalised: simulation.TossMarginalised,
		ElevenTotal: auctionElevenTotal{
			Total:        quantilesFrom(simulation.Total, auction.IntervalSourceSimulatorDraws),
			SpreadShare:  share,
			Samples:      simulation.Samples,
			SharedFactor: simulation.SharedFactor,
		},
	}, simulation.TotalDraws, nil
}

// candidateFromList finds the candidate on this auction's list.
//
// A projection is for a player in this room. One for a player nobody listed would be a
// number about an auction he is not in, and the operator's remedy — list him — is what the
// refusal says.
func candidateFromList(record *auction.Auction, playerID int64) (auction.NamedPlayer, error) {
	for _, player := range record.Players {
		if player.PlayerID == playerID {
			return auction.NamedPlayer{
				PlayerID:   player.PlayerID,
				ExternalID: player.ExternalID,
				PlayerName: player.PlayerName,
			}, nil
		}
	}
	return auction.NamedPlayer{}, &candidateNotListedError{PlayerID: playerID, AuctionID: record.ID}
}

// candidateNotListedError reports a projection asked for a player this auction does not
// hold.
type candidateNotListedError struct {
	PlayerID  int64
	AuctionID string
}

func (e *candidateNotListedError) Error() string {
	return fmt.Sprintf("player %d is not on auction %s's list, and a projection is for a player in this room",
		e.PlayerID, e.AuctionID)
}

// projectionSides assembles the two elevens the projection is made for, refusing each
// missing assumption by name.
func projectionSides(
	record *auction.Auction,
	candidate auction.NamedPlayer,
) ([]auction.NamedPlayer, *auction.Opposition, error) {
	if len(record.LikelyXI) == 0 {
		return nil, nil, auction.ErrNoLikelyEleven
	}
	if record.Opposition == nil || len(record.Opposition.Players) == 0 {
		return nil, nil, auction.ErrNoOpposition
	}
	if len(record.VenueIDs) == 0 {
		return nil, nil, auction.ErrNoGrounds
	}
	eleven, err := auction.ProjectedEleven(record.LikelyXI, candidate)
	if err != nil {
		return nil, nil, err
	}
	if len(record.Opposition.Players) != auction.TeamSize {
		return nil, nil, &auction.IncompleteElevenError{
			Have: len(record.Opposition.Players), Need: auction.TeamSize,
		}
	}
	if err := auction.RefuseSharedPlayers(eleven, record.Opposition.Players); err != nil {
		return nil, nil, err
	}
	return eleven, record.Opposition, nil
}

// forecastFor picks the candidate's own row out of the two elevens' forecasts.
//
// The candidate's eleven is team1 on every projection, so his row is the one on side 1: the
// response is one flat list for both sides, and matching on the registry id alone would let
// an opposition row answer for him (GO-04).
func forecastFor(
	forecast *predictteam.XIPerformanceResult,
	candidate auction.NamedPlayer,
) (*auctionCandidateForecast, error) {
	for _, player := range forecast.Players {
		if player.PlayerKey != candidate.ExternalID || player.Side != predictteam.Team1Side {
			continue
		}
		return &auctionCandidateForecast{
			Runs:         quantilesFromRange(player.Runs),
			BallsFaced:   quantilesFromRange(player.BallsFaced),
			RunsConceded: quantilesFromRange(player.RunsConceded),
			Wickets: auctionWicketDistribution{
				Expected: player.Wickets,
				P0:       player.WicketsP0,
				P1:       player.WicketsP1,
				P2Plus:   player.WicketsP2Plus,
				Note:     wicketsHaveNoIntervalNote,
			},
			InningsMarginalised: forecast.InningsMarginalised,
		}, nil
	}
	// A row left at zeros would read as a forecast of nothing rather than as a missing
	// forecast, which is the substitution §8.7 forbids.
	return nil, fmt.Errorf("the performance model returned no forecast for the candidate %q", candidate.ExternalID)
}

func quantilesFromRange(r predictteam.XISimulatedRange) auctionQuantiles {
	return auctionQuantiles{
		Q10:            r.P10,
		Median:         r.Median,
		Q90:            r.P90,
		IntervalSource: auction.IntervalSourceL2BQuantiles,
	}
}

func quantilesFrom(q auction.Quantiles, source string) auctionQuantiles {
	return auctionQuantiles{Q10: q.Q10, Median: q.Median, Q90: q.Q90, IntervalSource: source}
}

// mixtureOverGrounds pools the grounds' draws with the operator's weights.
//
// Absent weights mean no mixture, not an equal one: how often an eleven plays where is a
// fact about a season nobody has entered, and inventing a uniform mix would be this service
// asserting it.
func mixtureOverGrounds(
	weights []auctionVenueWeight,
	drawsByVenue map[int64][]float64,
) (*auctionMixture, error) {
	if len(weights) == 0 {
		return nil, nil
	}
	components := make([]auction.MixtureComponent, 0, len(weights))
	for _, weight := range weights {
		draws, ok := drawsByVenue[weight.VenueID]
		if !ok {
			return nil, &unknownMixtureGroundError{VenueID: weight.VenueID}
		}
		if weight.Weight < 0 {
			return nil, fmt.Errorf("a venue weight is not negative, and ground %d was sent %g",
				weight.VenueID, weight.Weight)
		}
		components = append(components, auction.MixtureComponent{Draws: draws, Weight: weight.Weight})
	}
	quantiles, err := auction.MixtureQuantiles(components)
	if err != nil {
		return nil, err
	}
	mixed := append([]auctionVenueWeight(nil), weights...)
	sort.Slice(mixed, func(i, j int) bool { return mixed[i].VenueID < mixed[j].VenueID })
	return &auctionMixture{
		Total:   quantilesFrom(quantiles, auction.IntervalSourceSimulatorDraws),
		Weights: mixed,
		Note:    mixtureNote,
	}, nil
}

// unknownMixtureGroundError reports a weight for a ground this auction is not for.
type unknownMixtureGroundError struct{ VenueID int64 }

func (e *unknownMixtureGroundError) Error() string {
	return fmt.Sprintf("venue %d is not one of this auction's grounds, so there are no draws to weight for it",
		e.VenueID)
}

func tossName(team1BatsFirst *bool) string {
	if team1BatsFirst == nil {
		return auctionTossUnknown
	}
	if *team1BatsFirst {
		return auctionTossBatFirst
	}
	return auctionTossChasing
}

func namedPlayerOnWire(player auction.NamedPlayer) auctionProjectionPlayer {
	return auctionProjectionPlayer{PlayerID: player.PlayerID, PlayerName: player.PlayerName}
}

func namedPlayersOnWire(players []auction.NamedPlayer) []auctionProjectionPlayer {
	out := make([]auctionProjectionPlayer, 0, len(players))
	for _, player := range players {
		out = append(out, namedPlayerOnWire(player))
	}
	return out
}

// intervalSourcesOnWire names the two intervals this answer carries, with the open defects
// against each. B-11 and B-14 are named here rather than left to the reader, because both
// are properties of the number beside them and neither is fixed by this item.
func intervalSourcesOnWire() []auctionIntervalSource {
	return []auctionIntervalSource{
		{
			Source: auction.IntervalSourceL2BQuantiles,
			Label:  "L2-B's quantiles",
			Note: "The performance model's own 10th and 90th percentile heads for one player, reported " +
				"rather than drawn from. At nominal coverage on the harness (plan §8.2).",
		},
		{
			Source:  auction.IntervalSourceSimulatorDraws,
			Label:   "The simulator's draws",
			Caveats: []string{"B-11", "B-14"},
			Note: "The 10th and 90th percentiles of whole matches drawn from the eleven's forecasts. " +
				"B-11 is open: one dispersion is fitted to two populations, so this interval is too narrow " +
				"by day and too wide at night — T20 first-innings coverage 0.734 / 0.841 at a nominal 0.80, " +
				"with six gated arms recorded as nulls. B-14 is open: the performance artifact is not " +
				"shape-checked at load, so an older calibration can restore silently — the run id shown is " +
				"the run that answered, not a promise about the shape of its calibration. Nothing here is " +
				"widened, narrowed or adjusted.",
		},
	}
}

// respondAuctionProjectionErr answers a refused projection, naming every assumption that
// is missing or wrong for what it is. Every one shows no number (§8.7, the gate's clause 6).
func respondAuctionProjectionErr(w http.ResponseWriter, auctionID string, err error) {
	var incomplete *auction.IncompleteElevenError
	if errors.As(err, &incomplete) {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "XI_INCOMPLETE",
			Message: incomplete.Error(),
			Hint: "set the likely eleven with PUT /api/auctions/{id}/assumptions: ten named players " +
				"and the candidate makes eleven, and the opposition is eleven of its own",
		})
		return
	}
	var sharedPlayer *auction.SharedPlayerError
	if errors.As(err, &sharedPlayer) {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "XI_PLAYER_ON_BOTH_SIDES",
			Message: sharedPlayer.Error(),
			Hint: "the likely eleven and the opposition are two different elevens; " +
				"change one of them with PUT /api/auctions/{id}/assumptions",
		})
		return
	}
	var unregistered *auction.UnregisteredPlayerError
	if errors.As(err, &unregistered) {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "XI_PLAYER_UNKNOWN",
			Message: unregistered.Error(),
			Hint:    "a player with no Cricsheet registry id cannot be projected; name another in his place",
		})
		return
	}
	var notListed *candidateNotListedError
	if errors.As(err, &notListed) {
		writeJSON(w, http.StatusNotFound, apiError{
			Code:    "CANDIDATE_NOT_LISTED",
			Message: notListed.Error(),
			Hint:    "list him first with POST /api/auctions/{id}/players",
		})
		return
	}
	var unknownGround *unknownMixtureGroundError
	if errors.As(err, &unknownGround) {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_PARAM",
			Message: unknownGround.Error(),
			Hint:    "the venue mix weights the auction's own grounds; they are on the record as venue_ids",
		})
		return
	}
	if errors.Is(err, auction.ErrNoLikelyEleven) ||
		errors.Is(err, auction.ErrNoOpposition) ||
		errors.Is(err, auction.ErrNoGrounds) {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "ASSUMPTIONS_INCOMPLETE",
			Message: err.Error(),
			Hint: "a projection is conditional on an eleven, an opposition and grounds, all named: " +
				"set the first two with PUT /api/auctions/{id}/assumptions and the grounds when the auction is created",
		})
		return
	}
	var runChanged *predictteam.ServedRunChangedError
	if errors.As(err, &runChanged) {
		writeJSON(w, http.StatusConflict, apiError{
			Code:    "SERVED_RUN_CHANGED",
			Message: runChanged.Error(),
			Hint:    "a reload landed while this projection was being assembled; run it again",
		})
		return
	}
	slog.Error("auction projection failed", slog.String("auction_id", auctionID), slog.Any("err", err))
	respondErr(w, err)
}
