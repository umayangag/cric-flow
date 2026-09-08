package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// The projection's assumptions and the eleven a side last fielded, against a real database
// (P3-2).
//
// These two are SQL and nothing else: a write that replaces an eleven rather than merging
// into it, and a read that finds a side's most recent *full* eleven and skips a partial
// team sheet. A mock over the pool would assert this file's own strings.
//
// They truncate. Run them against a scratch database: `make -C go-app test-db`.

// seedFieldedEleven records one side fielding an eleven in a T20 match, plus a newer match
// whose sheet holds only nine — the case the read must skip rather than offer.
func seedFieldedEleven(t *testing.T, fixture auctionFixture) []int64 {
	t.Helper()
	ctx := fixture.ctx
	formatID, err := db.GetOrCreateMatchFormat(ctx, "T20")
	require.NoError(t, err)

	playerIDs := make([]int64, 0, 11)
	for i := 0; i < 11; i++ {
		playerIDs = append(playerIDs, insertPlayerRow(
			ctx, t, fmt.Sprintf("fld%03x", i), fmt.Sprintf("Fielded Player %02d", i)))
	}
	// The older match with the full eleven, and a newer one whose sheet is short: "last"
	// must mean the last *complete* one, or an operator seeds an assumption from a side
	// that never played.
	insertFieldedMatch(t, fixture, formatID, 5001, "2026-05-24", playerIDs)
	insertFieldedMatch(t, fixture, formatID, 5002, "2026-05-30", playerIDs[:9])
	return playerIDs
}

func insertFieldedMatch(
	t *testing.T,
	fixture auctionFixture,
	formatID, matchID int64,
	matchDate string,
	playerIDs []int64,
) {
	t.Helper()
	require.NoError(t, db.Exec(fixture.ctx,
		`INSERT INTO match (match_id, format_id, match_date, original_match_type, event_name)
		 VALUES ($1, $2, $3, 'T20', 'Indian Premier League')`, matchID, formatID, matchDate))
	for _, playerID := range playerIDs {
		require.NoError(t, db.Exec(fixture.ctx,
			`INSERT INTO match_player (match_id, player_id, opposition_id) VALUES ($1, $2, $3)`,
			matchID, playerID, fixture.rivalID))
	}
}

func TestSetAssumptions_ReplacesEachElevenAndCarriesItBackOnTheRecord_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	opposition := seedFieldedEleven(t, fixture)
	router, _ := newAuctionOnRecord(t, fixture)
	auctionID := openAuctionID(t, router)

	path := fmt.Sprintf("/api/auctions/%s/assumptions", auctionID)
	status, written := callAuction(t, router, http.MethodPut, path, map[string]any{
		"likely_xi":  fixture.playerIDs[:4],
		"opposition": map[string]any{"club_id": fixture.rivalID, "player_ids": opposition},
	})

	require.Equal(t, http.StatusOK, status)
	assert.Len(t, written.Auction.LikelyXI, 4)
	require.NotNil(t, written.Auction.Opposition)
	assert.Equal(t, fixture.rivalID, written.Auction.Opposition.ClubID)
	assert.Len(t, written.Auction.Opposition.Players, 11)

	// A shorter eleven replaces the longer one rather than merging into it: a merge would
	// leave the record holding a player the operator has just removed.
	_, shortened := callAuction(t, router, http.MethodPut, path, map[string]any{
		"likely_xi": fixture.playerIDs[:2],
	})
	assert.Len(t, shortened.Auction.LikelyXI, 2)
	require.NotNil(t, shortened.Auction.Opposition,
		"a write that names one assumption leaves the other exactly as it stands")
	assert.Len(t, shortened.Auction.Opposition.Players, 11)

	// And it survives a reload, which is what the record is for.
	_, reread := callAuction(t, router, http.MethodGet, "/api/auctions/"+auctionID, nil)
	assert.Len(t, reread.Auction.LikelyXI, 2)
	require.NotNil(t, reread.Auction.Opposition)
	assert.Len(t, reread.Auction.Opposition.Players, 11)
}

func TestOppositionSuggestion_OffersTheLastCompleteElevenAndNotThePartialSheet_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	seedFieldedEleven(t, fixture)
	router, _ := newAuctionOnRecord(t, fixture)

	recorder := doAuctionRequest(t, router, http.MethodGet, fmt.Sprintf(
		"/api/auctions/%s/opposition-suggestion?club_id=%d", openAuctionID(t, router), fixture.rivalID), nil)

	require.Equal(t, http.StatusOK, recorder.Code)
	var answer oppositionSuggestion
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
	assert.Len(t, answer.Players, 11)
	assert.Equal(t, "2026-05-24", answer.FromMatch.MatchDate,
		"the newer sheet holds nine players, and a nine-man record is not an eleven anybody fielded")
	assert.Equal(t, "Indian Premier League", answer.FromMatch.EventName)
	assert.Contains(t, answer.Note, "starting point")
}

func TestOppositionSuggestion_RefusesASideWithNoRecordedEleven_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	router, _ := newAuctionOnRecord(t, fixture)

	recorder := doAuctionRequest(t, router, http.MethodGet, fmt.Sprintf(
		"/api/auctions/%s/opposition-suggestion?club_id=%d", openAuctionID(t, router), fixture.buyerID), nil)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "NO_FIELDED_ELEVEN")
}

func TestProjection_IsRefusedWhileAnAssumptionIsMissing_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	router, _ := newAuctionOnRecord(t, fixture)

	recorder := doAuctionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/api/auctions/%s/projection", openAuctionID(t, router)),
		map[string]any{"player_id": fixture.playerIDs[0]})

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "ASSUMPTIONS_INCOMPLETE")
	assert.NotContains(t, recorder.Body.String(), "q10",
		"a projection with an unnamed assumption shows no number")
}

// openAuctionID reads the id of the one auction on the record.
func openAuctionID(t *testing.T, router http.Handler) string {
	t.Helper()
	recorder := doAuctionRequest(t, router, http.MethodGet, "/api/auctions", nil)
	require.Equal(t, http.StatusOK, recorder.Code)
	var index struct {
		Auctions []auctionSummary `json:"auctions"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &index))
	require.Len(t, index.Auctions, 1)
	return index.Auctions[0].ID
}
