package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
	"github.com/umayangag/cric-flow/go-app/internal/auction/mocks"
	"github.com/umayangag/cric-flow/go-app/internal/db"
)

// The projection endpoint against a scripted ml-service (P3-2).
//
// Two things are asserted here that nothing else can assert: that the numbers on the
// answer are the ones the two ml-service calls returned, unaltered — no widening, no
// narrowing, no arithmetic of this service's own — and that no win probability and no
// marginal value is anywhere in the payload, under any key, with `/xi/optimize` never
// reached. The second is the rule the whole module is built on, and inspection is not
// enough for it: it is asserted through the client, against the rendered bytes.

// projectionAuction is the record the projection is made against: ten named players in the
// likely eleven, a named opposition of eleven, and two grounds.
func projectionAuction() auction.Auction {
	record := storedAuction()
	record.VenueIDs = []int64{4, 9}
	record.LikelyXI = projectionSide(100, 10)
	record.Opposition = &auction.Opposition{
		OppositionID: 22,
		Name:         "Rival Franchise",
		Players:      projectionSide(200, auction.TeamSize),
	}
	return record
}

// projectionSide builds n players whose registry ids are predictable, so a scripted
// ml-service can answer for exactly the ids it was sent.
func projectionSide(base int64, n int) []auction.NamedPlayer {
	players := make([]auction.NamedPlayer, 0, n)
	for i := 0; i < n; i++ {
		id := base + int64(i)
		players = append(players, auction.NamedPlayer{
			PlayerID:   id,
			ExternalID: fmt.Sprintf("reg%d", id),
			PlayerName: fmt.Sprintf("Player %d", id),
		})
	}
	return players
}

// stubLookups answers the projection's database reads without a database.
type stubLookups struct {
	venueNames map[int64]string
	eleven     *db.LastFieldedEleven
	elevenErr  error
}

func (s stubLookups) VenueName(_ context.Context, venueID int64) (string, error) {
	return s.venueNames[venueID], nil
}

func (s stubLookups) LastFieldedEleven(_ context.Context, _ string, _ int64) (*db.LastFieldedEleven, error) {
	return s.eleven, s.elevenErr
}

func projectionLookups() stubLookups {
	return stubLookups{venueNames: map[int64]string{4: "Chepauk", 9: "Wankhede"}}
}

// scriptedForecast is what the fake ml-service answers for one ground, so a test can name
// the numbers it then expects to see on the wire.
type scriptedForecast struct {
	candidateRuns  [3]float64
	wickets        [4]float64
	totalQuantiles [3]float64
	totalDraws     []float64
	venueN         float64
	venueBFRate    float64
	spreadShare    float64
}

// projectionMLService answers /performance/predict and /simulate per venue and records
// every path it was asked for — including any that should never be asked.
func projectionMLService(
	t *testing.T,
	byVenue map[int64]scriptedForecast,
	paths *[]string,
	sent *[]map[string]any,
) *MLClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		*sent = append(*sent, body)

		venueID := int64(body["venue_id"].(float64))
		script, ok := byVenue[venueID]
		require.True(t, ok, "the fake ml-service was asked about venue %d, which no test scripted", venueID)
		team1 := body["team1_player_ids"].([]any)
		_, tossKnown := body["team1_bats_first"]

		w.Header().Set("Content-Type", "application/json")
		served := map[string]string{"run_id": "20260906T083819Z-36689f80", "ratings_through": "2026-09-02"}
		if r.URL.Path == "/performance/predict" {
			players := make([]map[string]any, 0, len(team1))
			for _, key := range team1 {
				players = append(players, map[string]any{
					"player_id": key,
					// The candidate's eleven is team1 on every projection, and his row is
					// read by side as well as by id (GO-04).
					"side": 1,
					"runs": map[string]float64{
						"q10": script.candidateRuns[0], "median": script.candidateRuns[1], "q90": script.candidateRuns[2],
					},
					"balls_faced":   map[string]float64{"q10": 8, "median": 18, "q90": 30},
					"runs_conceded": map[string]float64{"q10": 0, "median": 12, "q90": 34},
					"wickets": map[string]float64{
						"expected": script.wickets[0], "p0": script.wickets[1],
						"p1": script.wickets[2], "p2_plus": script.wickets[3],
					},
				})
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"players":              players,
				"innings_marginalised": !tossKnown,
				"venue_context": map[string]any{
					"venue_bf_rate": script.venueBFRate,
					"venue_n":       script.venueN,
					"neutral":       script.venueN == 0,
				},
				"served_ratings": served,
			}))
			return
		}
		require.Equal(t, "/simulate", r.URL.Path)
		require.Equal(t, true, body["return_total_draws"],
			"the mixture is inverted from the draws, so the projection always asks for them")
		players := make([]map[string]any, 0, len(team1))
		for _, key := range team1 {
			players = append(players, map[string]any{"player_id": key, "spread_share": script.spreadShare})
		}
		// The win probability and the margin are answered exactly as ml-service answers
		// them. Nothing may carry them onto the wire, and this is what makes that a real
		// assertion rather than a fake that could not have failed.
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"n_samples":         len(script.totalDraws),
			"toss_marginalised": !tossKnown,
			"shared_factor":     true,
			"team1": map[string]any{
				"total": map[string]float64{
					"q10": script.totalQuantiles[0], "median": script.totalQuantiles[1], "q90": script.totalQuantiles[2],
				},
				"players":     players,
				"total_draws": script.totalDraws,
			},
			"team2": map[string]any{
				"total":       map[string]float64{"q10": 130, "median": 160, "q90": 190},
				"players":     []map[string]any{},
				"total_draws": script.totalDraws,
			},
			"win_probability": map[string]any{
				"simulated": 0.61, "p_tie": 0.01, "display": 0.58,
				"headline": 0.58, "headline_source": "display",
			},
			"margin":         map[string]any{"p_bat_first_wins": 0.5, "p_chaser_wins": 0.49, "p_tie": 0.01},
			"served_ratings": served,
		}))
	}))
	t.Cleanup(server.Close)
	return &MLClient{BaseURL: server.URL, HTTP: server.Client()}
}

// twoGroundScript is the answers both grounds give: Chepauk with context, the Wankhede
// with none, so the neutral row can be asserted.
func twoGroundScript() map[int64]scriptedForecast {
	return map[int64]scriptedForecast{
		4: {
			candidateRuns:  [3]float64{6, 24, 51},
			wickets:        [4]float64{0.42, 0.68, 0.24, 0.08},
			totalQuantiles: [3]float64{141, 172, 205},
			totalDraws:     []float64{141, 150, 160, 172, 180, 190, 200, 205},
			venueN:         83,
			venueBFRate:    0.47,
			spreadShare:    0.12,
		},
		9: {
			candidateRuns:  [3]float64{5, 22, 48},
			wickets:        [4]float64{0.40, 0.70, 0.23, 0.07},
			totalQuantiles: [3]float64{139, 168, 201},
			totalDraws:     []float64{139, 148, 158, 168, 178, 188, 198, 201},
			venueN:         0,
			venueBFRate:    0.5,
			spreadShare:    0.11,
		},
	}
}

// projectThrough runs POST /api/auctions/{id}/projection against the app.
func projectThrough(t *testing.T, app *App, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	request := mux.SetURLVars(
		httptest.NewRequest(http.MethodPost, "/api/auctions/auction-1/projection", bytes.NewReader(raw)),
		map[string]string{"id": "auction-1"},
	)
	recorder := httptest.NewRecorder()
	app.projectAuctionCandidateHandler(recorder, request)
	return recorder
}

func decodeProjection(t *testing.T, recorder *httptest.ResponseRecorder) auctionProjectionResponse {
	t.Helper()
	var answer auctionProjectionResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
	return answer
}

// appWithProjectionAuction wires a mock store holding the record with the assumptions set.
func appWithProjectionAuction(t *testing.T, paths *[]string, sent *[]map[string]any) *App {
	t.Helper()
	store := mocks.NewMockStore(t)
	store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(projectionAuction()), nil)
	return &App{
		auctionStore:   store,
		auctionLookups: projectionLookups(),
		mlClient:       projectionMLService(t, twoGroundScript(), paths, sent),
	}
}

func TestProjectAuctionCandidate_ShowsTheServedNumbersPerGroundWithBothIntervalsNamed(t *testing.T) {
	var paths []string
	var sent []map[string]any
	app := appWithProjectionAuction(t, &paths, &sent)

	recorder := projectThrough(t, app, map[string]any{"player_id": 2})

	require.Equal(t, http.StatusOK, recorder.Code)
	answer := decodeProjection(t, recorder)
	require.Len(t, answer.Grounds, 2)

	chepauk := answer.Grounds[0]
	assert.Equal(t, int64(4), chepauk.VenueID)
	assert.Equal(t, "Chepauk", chepauk.VenueName)
	assert.Equal(t, 6.0, chepauk.Candidate.Runs.Q10)
	assert.Equal(t, 24.0, chepauk.Candidate.Runs.Median)
	assert.Equal(t, 51.0, chepauk.Candidate.Runs.Q90,
		"the quantiles are L2-B's as served: nothing widened, narrowed or adjusted")
	assert.Equal(t, auction.IntervalSourceL2BQuantiles, chepauk.Candidate.Runs.IntervalSource)
	assert.Equal(t, 172.0, chepauk.ElevenTotal.Total.Median)
	assert.Equal(t, auction.IntervalSourceSimulatorDraws, chepauk.ElevenTotal.Total.IntervalSource)
	assert.Equal(t, 0.12, chepauk.ElevenTotal.SpreadShare)
	assert.Equal(t, 8, chepauk.ElevenTotal.Samples)

	assert.Equal(t, 0.42, chepauk.Candidate.Wickets.Expected)
	assert.Equal(t, 0.68, chepauk.Candidate.Wickets.P0)
	assert.Contains(t, chepauk.Candidate.Wickets.Note, "no q10 or q90",
		"wickets have no quantiles on this path, and none is derived from the probabilities")

	assert.Equal(t, "20260906T083819Z-36689f80", answer.ServedRatings.RunID)
	assert.Equal(t, "2026-09-02", answer.ServedRatings.RatingsThrough)
	require.Len(t, answer.Intervals, 2)
	assert.Equal(t, []string{"B-11", "B-14"}, answer.Intervals[1].Caveats,
		"the simulator's interval names both open defects where it is shown")
	assert.Empty(t, answer.Intervals[0].Caveats, "L2-B's quantiles are at nominal coverage on the harness")
}

func TestProjectAuctionCandidate_SaysAGroundTheServedStateHasNoContextForReadsNeutral(t *testing.T) {
	var paths []string
	var sent []map[string]any
	app := appWithProjectionAuction(t, &paths, &sent)

	answer := decodeProjection(t, projectThrough(t, app, map[string]any{"player_id": 2}))

	require.Len(t, answer.Grounds, 2)
	assert.False(t, answer.Grounds[0].Ground.Neutral)
	assert.Equal(t, 83.0, answer.Grounds[0].Ground.Matches)
	assert.Empty(t, answer.Grounds[0].Ground.Note)

	wankhede := answer.Grounds[1]
	assert.True(t, wankhede.Ground.Neutral)
	assert.Equal(t, 0.0, wankhede.Ground.Matches)
	assert.Contains(t, wankhede.Ground.Note, "read at the prior",
		"§8.7: a ground the model has nothing for is said to read neutral, per ground")
	assert.Contains(t, answer.Assumptions.WhatAGroundChanges, "venue_bf_rate, venue_n")
	assert.Contains(t, answer.Assumptions.WhatAGroundChanges, "A-1")
}

func TestProjectAuctionCandidate_CarriesTheThreeAssumptionsBackOnTheAnswer(t *testing.T) {
	var paths []string
	var sent []map[string]any
	app := appWithProjectionAuction(t, &paths, &sent)

	answer := decodeProjection(t, projectThrough(t, app, map[string]any{"player_id": 2}))

	require.Len(t, answer.Assumptions.Eleven, 11)
	assert.Equal(t, int64(2), answer.Assumptions.Eleven[10].PlayerID,
		"the candidate is the eleventh man, and the eleven on the answer is the one that was sent")
	assert.Equal(t, int64(22), answer.Assumptions.Opposition.ClubID)
	assert.Equal(t, "Rival Franchise", answer.Assumptions.Opposition.Name)
	require.Len(t, answer.Assumptions.Opposition.Players, 11)
	assert.Equal(t, []auctionGroundRef{
		{VenueID: 4, VenueName: "Chepauk"},
		{VenueID: 9, VenueName: "Wankhede"},
	}, answer.Assumptions.Grounds)
	assert.Equal(t, auctionTossUnknown, answer.Assumptions.Toss)
	assert.Contains(t, answer.Assumptions.NotXIPicking, "rating order")
}

func TestProjectAuctionCandidate_MarginalisesTheTossUnlessTheOperatorSetsIt(t *testing.T) {
	testCases := []struct {
		name     string
		body     map[string]any
		wantToss string
		wantSent any
	}{
		{
			name:     "no toss is the marginal question a projection is asked months early",
			body:     map[string]any{"player_id": 2},
			wantToss: auctionTossUnknown,
			wantSent: nil,
		},
		{
			name:     "batting first is a different question and is sent as one",
			body:     map[string]any{"player_id": 2, "team1_bats_first": true},
			wantToss: auctionTossBatFirst,
			wantSent: true,
		},
		{
			name:     "chasing likewise",
			body:     map[string]any{"player_id": 2, "team1_bats_first": false},
			wantToss: auctionTossChasing,
			wantSent: false,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			var paths []string
			var sent []map[string]any
			app := appWithProjectionAuction(t, &paths, &sent)

			answer := decodeProjection(t, projectThrough(t, app, testCase.body))

			assert.Equal(t, testCase.wantToss, answer.Assumptions.Toss)
			require.NotEmpty(t, sent)
			assert.Equal(t, testCase.wantSent, sent[0]["team1_bats_first"])
			assert.Equal(t, testCase.wantSent == nil, answer.Grounds[0].TossMarginalised)
		})
	}
}

func TestProjectAuctionCandidate_MixesTheGroundsFromTheDrawsWhenTheOperatorWeightsThem(t *testing.T) {
	var paths []string
	var sent []map[string]any
	app := appWithProjectionAuction(t, &paths, &sent)

	answer := decodeProjection(t, projectThrough(t, app, map[string]any{
		"player_id": 2,
		"venue_weights": []map[string]any{
			{"venue_id": 4, "weight": 0.5},
			{"venue_id": 9, "weight": 0.5},
		},
	}))

	require.NotNil(t, answer.Mixture)
	assert.Equal(t, auction.IntervalSourceSimulatorDraws, answer.Mixture.Total.IntervalSource)
	assert.Contains(t, answer.Mixture.Note, "not the average of the per-ground ranges")
	// Sixteen draws at equal mass (0.0625 each). Sorted, they run 139, 141, 148, 150, 158,
	// 160, 168, 172, 178, 180, 188, 190, 198, 200, 201, 205 — so the median is the eighth
	// (172), q10 the second (141) and q90 the fifteenth (201). The mean of the two grounds'
	// own medians would be 170, a total neither ground drew.
	assert.Equal(t, 141.0, answer.Mixture.Total.Q10)
	assert.Equal(t, 172.0, answer.Mixture.Total.Median)
	assert.Equal(t, 201.0, answer.Mixture.Total.Q90)
	assert.NotEqual(t, 170.0, answer.Mixture.Total.Median,
		"a mixture's quantiles are not the mean of its parts', which is why the draws are pooled")
}

func TestProjectAuctionCandidate_ShowsNoMixtureWhenNoMixWasNamed(t *testing.T) {
	var paths []string
	var sent []map[string]any
	app := appWithProjectionAuction(t, &paths, &sent)

	answer := decodeProjection(t, projectThrough(t, app, map[string]any{"player_id": 2}))

	assert.Nil(
		t,
		answer.Mixture,
		"how often an eleven plays where is a fact nobody has entered; a uniform mix would be this service asserting one",
	)
}

// TestProjectAuctionCandidate_CarriesNoWinProbabilityAndNoMarginalValue is the rule that is
// the item, asserted against the rendered bytes and through the client (plan §8.8).
//
// The scripted ml-service answers a `win_probability` and a `margin` exactly as ml-service
// does, so the assertion has something real to fail against: the Go type this is decoded
// into carries no such field, and the value therefore never exists in this process.
func TestProjectAuctionCandidate_CarriesNoWinProbabilityAndNoMarginalValue(t *testing.T) {
	var paths []string
	var sent []map[string]any
	app := appWithProjectionAuction(t, &paths, &sent)

	recorder := projectThrough(t, app, map[string]any{
		"player_id":     2,
		"venue_weights": []map[string]any{{"venue_id": 4, "weight": 1}},
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	for _, key := range everyKeyIn(payload) {
		assert.NotContains(t, strings.ToLower(key), "win",
			"a P(win) beside a purchase is the XI-picking claim in another coat")
		// `innings_marginalised` and `toss_marginalised` are the toss being averaged and are
		// not values of anything, so the assertion names the field the record forbids.
		assert.NotContains(t, strings.ToLower(key), "marginal_value",
			"nothing was maximised here, so nothing carries a marginal value")
	}
	assert.NotContains(t, strings.ToLower(recorder.Body.String()), "0.61",
		"the simulated win probability the fake service answered is nowhere in the payload")
	for _, path := range paths {
		assert.NotEqual(t, "/xi/optimize", path,
			"the module never asks the objective anything: T20 selection is rating-ordered (plan §8.8)")
	}
	assert.Equal(t, []string{
		"/performance/predict", "/simulate", "/performance/predict", "/simulate",
	}, paths, "one forecast and one simulation per ground, and nothing else")
}

// everyKeyIn walks a decoded payload and returns every object key in it, at any depth.
func everyKeyIn(node any) []string {
	switch typed := node.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key, value := range typed {
			keys = append(keys, key)
			keys = append(keys, everyKeyIn(value)...)
		}
		return keys
	case []any:
		var keys []string
		for _, value := range typed {
			keys = append(keys, everyKeyIn(value)...)
		}
		return keys
	default:
		return nil
	}
}

func TestProjectAuctionCandidate_RefusesEveryStateThatCannotBeProjectedAndShowsNoNumber(t *testing.T) {
	tenManXI := projectionAuction()
	tenManXI.LikelyXI = projectionSide(100, 9)

	noEleven := projectionAuction()
	noEleven.LikelyXI = nil

	noOpposition := projectionAuction()
	noOpposition.Opposition = nil

	noGrounds := projectionAuction()
	noGrounds.VenueIDs = nil

	unregistered := projectionAuction()
	unregistered.LikelyXI[3].ExternalID = ""

	testCases := []struct {
		name        string
		record      auction.Auction
		body        map[string]any
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "a ten-man side is refused exactly as the predict path refuses one",
			record:      tenManXI,
			body:        map[string]any{"player_id": 2},
			wantStatus:  http.StatusBadRequest,
			wantCode:    "XI_INCOMPLETE",
			wantMessage: "holds 10 players",
		},
		{
			name:        "no likely eleven is a missing assumption, named as one",
			record:      noEleven,
			body:        map[string]any{"player_id": 2},
			wantStatus:  http.StatusBadRequest,
			wantCode:    "ASSUMPTIONS_INCOMPLETE",
			wantMessage: "no likely eleven",
		},
		{
			name:        "a projection against no one is a projection for no league",
			record:      noOpposition,
			body:        map[string]any{"player_id": 2},
			wantStatus:  http.StatusBadRequest,
			wantCode:    "ASSUMPTIONS_INCOMPLETE",
			wantMessage: "no league",
		},
		{
			name:        "a projection is per ground",
			record:      noGrounds,
			body:        map[string]any{"player_id": 2},
			wantStatus:  http.StatusBadRequest,
			wantCode:    "ASSUMPTIONS_INCOMPLETE",
			wantMessage: "names no grounds",
		},
		{
			name:        "a player the registry does not know cannot be projected, and the ten who resolved are not scored",
			record:      unregistered,
			body:        map[string]any{"player_id": 2},
			wantStatus:  http.StatusBadRequest,
			wantCode:    "XI_PLAYER_UNKNOWN",
			wantMessage: "no registry id",
		},
		{
			name:        "a candidate nobody listed is not in this room",
			record:      projectionAuction(),
			body:        map[string]any{"player_id": 4242},
			wantStatus:  http.StatusNotFound,
			wantCode:    "CANDIDATE_NOT_LISTED",
			wantMessage: "not on auction auction-1's list",
		},
		{
			name:   "a mix naming a ground this auction is not for has no draws to weight",
			record: projectionAuction(),
			body: map[string]any{
				"player_id":     2,
				"venue_weights": []map[string]any{{"venue_id": 77, "weight": 1}},
			},
			wantStatus:  http.StatusBadRequest,
			wantCode:    "INVALID_PARAM",
			wantMessage: "not one of this auction's grounds",
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			store := mocks.NewMockStore(t)
			store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(testCase.record), nil)
			var paths []string
			var sent []map[string]any
			app := &App{
				auctionStore:   store,
				auctionLookups: projectionLookups(),
				mlClient:       projectionMLService(t, twoGroundScript(), &paths, &sent),
			}

			recorder := projectThrough(t, app, testCase.body)

			require.Equal(t, testCase.wantStatus, recorder.Code)
			var refusal apiError
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &refusal))
			assert.Equal(t, testCase.wantCode, refusal.Code)
			assert.Contains(t, refusal.Message, testCase.wantMessage)
			assert.NotEmpty(t, refusal.Hint, "a refusal says what the operator does next")
			assert.NotContains(t, recorder.Body.String(), "q10",
				"a refused projection shows no number at all")
		})
	}
}

func TestProjectAuctionCandidate_OnAStaleRegistryRelaysTheRefusalAndShowsNoNumber(t *testing.T) {
	store := mocks.NewMockStore(t)
	store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(projectionAuction()), nil)
	app := &App{
		auctionStore:   store,
		auctionLookups: projectionLookups(),
		mlClient: refusingMLService(t, http.StatusServiceUnavailable, "RATINGS_STALE",
			"ratings run through 2026-07-01 (69 days old, limit 14)"),
	}

	recorder := projectThrough(t, app, map[string]any{"player_id": 2})

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code,
		"ml-service's 'not now' is just as true of this service, which cannot project without it")
	var refusal apiError
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &refusal))
	assert.Equal(t, "RATINGS_STALE", refusal.Code)
	assert.Contains(t, refusal.Message, "69 days old")
	assert.NotContains(t, recorder.Body.String(), "q10")
}

func TestSetAuctionAssumptionsHandler_WritesEachAssumptionAndLeavesTheOtherAlone(t *testing.T) {
	elevenIDs := []int64{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210}
	tenIDs := []int64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109}
	store := mocks.NewMockStore(t)
	store.EXPECT().
		SetAssumptions(mock.Anything, "auction-1", mock.MatchedBy(func(change auction.AssumptionsChange) bool {
			return change.LikelyXIPlayerIDs != nil && len(*change.LikelyXIPlayerIDs) == 10 &&
				change.Opposition != nil && change.Opposition.OppositionID == 22
		})).
		Return(ptr(projectionAuction()), nil)
	var paths []string
	app := &App{auctionStore: store, mlClient: roleReadServer(t, &paths)}

	recorder := setAssumptionsThrough(t, app, map[string]any{
		"likely_xi":  tenIDs,
		"opposition": map[string]any{"club_id": 22, "player_ids": elevenIDs},
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	answer := decodeAuction(t, recorder)
	assert.Len(t, answer.Auction.LikelyXI, 10,
		"the assumptions come back on the record, so every later item reads one list")
	require.NotNil(t, answer.Auction.Opposition)
	assert.Equal(t, int64(22), answer.Auction.Opposition.ClubID)
}

func TestSetAuctionAssumptionsHandler_RefusesAWriteThatChangesNothingOrCannotBeProjectedUnder(t *testing.T) {
	testCases := []struct {
		name        string
		body        map[string]any
		wantMessage string
	}{
		{
			name:        "a body naming neither assumption changes nothing",
			body:        map[string]any{},
			wantMessage: "changes nothing as it stands",
		},
		{
			name: "a ten-man opposition is a side nobody plays",
			body: map[string]any{
				"opposition": map[string]any{"club_id": 22, "player_ids": []int64{1, 2, 3}},
			},
			wantMessage: "the opposition is an eleven",
		},
		{
			name:        "a likely eleven of twelve is not an eleven any candidate joins",
			body:        map[string]any{"likely_xi": []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
			wantMessage: "at most 11",
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			app := &App{auctionStore: mocks.NewMockStore(t)}

			recorder := setAssumptionsThrough(t, app, testCase.body)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			var refusal apiError
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &refusal))
			assert.Equal(t, "INVALID_PARAM", refusal.Code)
			assert.Contains(t, refusal.Message, testCase.wantMessage)
		})
	}
}

func setAssumptionsThrough(t *testing.T, app *App, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	request := mux.SetURLVars(
		httptest.NewRequest(http.MethodPut, "/api/auctions/auction-1/assumptions", bytes.NewReader(raw)),
		map[string]string{"id": "auction-1"},
	)
	recorder := httptest.NewRecorder()
	app.setAuctionAssumptionsHandler(recorder, request)
	return recorder
}

func TestOppositionSuggestionHandler_OffersTheLastRecordedElevenWithTheMatchItCameFrom(t *testing.T) {
	store := mocks.NewMockStore(t)
	store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(projectionAuction()), nil)
	lookups := projectionLookups()
	lookups.eleven = &db.LastFieldedEleven{
		OppositionID:   22,
		OppositionName: "Rival Franchise",
		MatchDate:      time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		EventName:      "Indian Premier League",
		VenueName:      "Chepauk",
		Players:        projectionSide(200, 11),
	}
	app := &App{auctionStore: store, auctionLookups: lookups}

	recorder := suggestionThrough(t, app, "22")

	require.Equal(t, http.StatusOK, recorder.Code)
	var answer oppositionSuggestion
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
	assert.Equal(t, int64(22), answer.ClubID)
	assert.Len(t, answer.Players, 11)
	assert.Equal(t, "2026-05-24", answer.FromMatch.MatchDate,
		"a fact's age is part of the fact: the operator decides from it whether to edit the side")
	assert.Contains(t, answer.Note, "starting point")
}

func TestOppositionSuggestionHandler_RefusesWhereNoElevenIsRecorded(t *testing.T) {
	store := mocks.NewMockStore(t)
	store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(projectionAuction()), nil)
	lookups := projectionLookups()
	lookups.elevenErr = db.ErrNoFieldedEleven
	app := &App{auctionStore: store, auctionLookups: lookups}

	recorder := suggestionThrough(t, app, "22")

	require.Equal(t, http.StatusNotFound, recorder.Code)
	var refusal apiError
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &refusal))
	assert.Equal(t, "NO_FIELDED_ELEVEN", refusal.Code)
	assert.Contains(t, refusal.Hint, "the operator names",
		"nothing is assembled for the operator: an invented side would be a guess presented as evidence")
}

func suggestionThrough(t *testing.T, app *App, clubID string) *httptest.ResponseRecorder {
	t.Helper()
	request := mux.SetURLVars(
		httptest.NewRequest(http.MethodGet, "/api/auctions/auction-1/opposition-suggestion?club_id="+clubID, nil),
		map[string]string{"id": "auction-1"},
	)
	recorder := httptest.NewRecorder()
	app.oppositionSuggestionHandler(recorder, request)
	return recorder
}
