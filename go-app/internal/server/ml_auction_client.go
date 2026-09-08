package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// The auction module's simulation call (P3-2).
//
// `/simulate` answers with a `win_probability` object. This client never decodes it.
//
// That is the point of the file existing at all. The rule the module is built on is that
// it is valuation and projection and never XI-picking, and a P(win) beside a purchase is
// the XI-picking claim in another coat: it says "buy him and your eleven wins more", which
// is the claim the record refutes — optimised selection in domestic T20 is
// indistinguishable from rating order (plan §8.8), and the IPL is domestic T20. Dropping
// it in the renderer would leave it one careless field away from the surface; dropping it
// at the JSON boundary means the value never exists in this process at all, and a test
// asserts the endpoint's payload holds no key containing "win".
//
// The rest of the response is taken whole: the totals as drawn, the candidate's own share
// of the total's spread, and — because a mixture over grounds cannot be computed from
// three summaries — the totals of every draw.

// mlAuctionSimulatedTotal is one side's total, as drawn. No mean, no sd, no median-band
// scorecard: the auction shows a total with its range and nothing that would read as a
// scorecard for a fixture that does not exist.
type mlAuctionSimulatedTotal struct {
	Q10    float64 `json:"q10"`
	Median float64 `json:"median"`
	Q90    float64 `json:"q90"`
}

type mlAuctionSimulatedPlayer struct {
	PlayerID    string  `json:"player_id"`
	SpreadShare float64 `json:"spread_share"`
}

type mlAuctionSimulatedSide struct {
	Total   mlAuctionSimulatedTotal    `json:"total"`
	Players []mlAuctionSimulatedPlayer `json:"players"`
	// TotalDraws is this side's total in every draw, asked for with `return_total_draws`.
	// The mixture over the auction's grounds is inverted from these pooled: a mixture's
	// quantiles are not the mean of its parts', so three summaries cannot be combined.
	TotalDraws []float64 `json:"total_draws"`
}

// mlAuctionSimulateResponse is `/simulate`'s answer as the auction reads it. Compare it
// with `mlSimulateResponse`: the fields absent here are absent on purpose — the win
// probability above all, and with it the margin, whose `p_bat_first_wins` is a win
// probability wearing the toss's clothes.
type mlAuctionSimulateResponse struct {
	NSamples         int                    `json:"n_samples"`
	TossMarginalised bool                   `json:"toss_marginalised"`
	SharedFactor     bool                   `json:"shared_factor"`
	Team1            mlAuctionSimulatedSide `json:"team1"`
	Team2            mlAuctionSimulatedSide `json:"team2"`
	ServedRatings    mlServedRatings        `json:"served_ratings"`
}

// AuctionSimulationRequest asks for one eleven's total at one ground.
type AuctionSimulationRequest struct {
	Format          string
	Team1PlayerKeys []string
	Team2PlayerKeys []string
	Team1ID         int64
	Team2ID         int64
	VenueID         int64
	// Team1BatsFirst is nil where the operator has not set the toss, and half the draws
	// then go each way.
	Team1BatsFirst *bool
	// Samples is the draw count. Zero takes the served default, which is what this module
	// always does: a projection drawn at a lower count to save time would be a narrower
	// answer to the same question, with nothing on it saying so.
	Samples int
	Seed    int
}

// AuctionSimulationResult is the eleven's total at one ground and nothing about who wins.
type AuctionSimulationResult struct {
	Samples          int
	TossMarginalised bool
	// SharedFactor says whether the simulator that drew this carried a shared match factor
	// (B-12): a format whose calibration fold was too thin ships the un-widened simulator,
	// whose intervals are a different population's.
	SharedFactor bool
	// Total is the projected eleven's — team1's, which is the side the candidate is in.
	Total auction.Quantiles
	// TotalDraws is that total in every draw, for pooling across grounds.
	TotalDraws []float64
	// SpreadShareByKey is each of the eleven's share of the total's spread, keyed by
	// registry id.
	SpreadShareByKey map[string]float64
	Served           predictteam.ServedRatings
}

// SimulateForAuction calls POST /simulate for one eleven at one ground, and decodes an
// answer that has no win probability in it.
func (c *MLClient) SimulateForAuction(
	ctx context.Context,
	req AuctionSimulationRequest,
) (*AuctionSimulationResult, error) {
	payload, err := json.Marshal(struct {
		mlSimulateRequest
		ReturnTotalDraws bool `json:"return_total_draws"`
	}{
		mlSimulateRequest: mlSimulateRequest{
			Format:         req.Format,
			Team1PlayerIDs: req.Team1PlayerKeys,
			Team2PlayerIDs: req.Team2PlayerKeys,
			Team1ID:        optionalID(req.Team1ID),
			Team2ID:        optionalID(req.Team2ID),
			VenueID:        optionalID(req.VenueID),
			Team1BatsFirst: req.Team1BatsFirst,
			NSamples:       req.Samples,
			Seed:           req.Seed,
		},
		ReturnTotalDraws: true,
	})
	if err != nil {
		return nil, err
	}
	var out mlAuctionSimulateResponse
	if err := c.postJSON(ctx, "/simulate", payload, &out); err != nil {
		return nil, err
	}
	// The toss the draws used has to be the toss that was asked for, or the request was
	// silently changed and the label on the answer is wrong (§8.7, the check the predict
	// path makes for the same reason).
	if out.TossMarginalised != (req.Team1BatsFirst == nil) {
		return nil, fmt.Errorf(
			"simulate for auction: the toss was %s but the simulator reports toss_marginalised=%t",
			tossDescriptionForAuction(req.Team1BatsFirst), out.TossMarginalised)
	}
	if len(out.Team1.TotalDraws) == 0 {
		return nil, fmt.Errorf(
			"simulate for auction: the simulator returned no draws, and a mixture over grounds is computed from them")
	}
	shares := make(map[string]float64, len(out.Team1.Players))
	for _, player := range out.Team1.Players {
		shares[player.PlayerID] = player.SpreadShare
	}
	return &AuctionSimulationResult{
		Samples:          out.NSamples,
		TossMarginalised: out.TossMarginalised,
		SharedFactor:     out.SharedFactor,
		Total: auction.Quantiles{
			Q10:    out.Team1.Total.Q10,
			Median: out.Team1.Total.Median,
			Q90:    out.Team1.Total.Q90,
		},
		TotalDraws:       out.Team1.TotalDraws,
		SpreadShareByKey: shares,
		Served:           out.ServedRatings.served(),
	}, nil
}

// tossDescriptionForAuction names the batting order for an error message, in the buyer's
// terms rather than the wire's: the candidate's side is team1 on every projection.
func tossDescriptionForAuction(team1BatsFirst *bool) string {
	if team1BatsFirst == nil {
		return "unknown"
	}
	if *team1BatsFirst {
		return "the candidate's eleven bats first"
	}
	return "the candidate's eleven chases"
}
