package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/db/dbtest"
)

// The auction record end to end against a real database and a scripted role read (P3-1).
//
// The state transitions are the item's gate — an auction survives a reload as entered,
// every sale and unsold outcome is on the record with its price and buyer, and an undo
// returns a player to available — and they are SQL: a CHECK constraint that refuses half a
// sale, an update that clears the buyer and the price with the state. A mock over the pool
// would assert that this file's own strings were sent.
//
// These truncate. Run them against a scratch database, never one holding an import:
// `make -C go-app test-db`; dbtest.SkipUnlessScratchDatabase refuses the working one.

// auctionFixture is one franchise, three rival buyers and six listed players.
type auctionFixture struct {
	ctx       context.Context
	buyerID   int64
	rivalID   int64
	playerIDs []int64
	// externalIDs are the registry ids the scripted role read answers under, in the same
	// order as playerIDs.
	externalIDs []string
}

// testAPIKey unlocks the admin subrouter for these tests. They run through the real
// router rather than calling bare handlers, because the path variable an auction is named
// by is the router's and a handler called directly would never see it.
const testAPIKey = "auction-integration-key"

func seedAuctionFixture(t *testing.T) auctionFixture {
	t.Helper()
	t.Setenv("API_KEY", testAPIKey)
	ctx := context.Background()
	pool, err := db.Connect(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })
	_, file, _, _ := runtime.Caller(0)
	require.NoError(t, db.RunMigrations(ctx, filepath.Clean(filepath.Join(filepath.Dir(file), "../../migrations"))))
	require.NoError(t, db.Exec(ctx, `TRUNCATE TABLE
		auction_player, auction_venue, auction_likely_xi,
		auction_opposition_player, auction_opposition, auction,
		issued_prediction,
		player_status_event, player_status, player_biography,
		ball_event, match_player, batting_data, bowling_data,
		fielding_data, fielding_event, match_inning, match, player, opposition RESTART IDENTITY`))

	fixture := auctionFixture{ctx: ctx}
	fixture.buyerID = insertClub(ctx, t, "Buying Franchise")
	fixture.rivalID = insertClub(ctx, t, "Rival Franchise")
	for i := 0; i < 6; i++ {
		externalID := fmt.Sprintf("auc%03x", i)
		fixture.externalIDs = append(fixture.externalIDs, externalID)
		fixture.playerIDs = append(
			fixture.playerIDs,
			insertPlayerRow(ctx, t, externalID, fmt.Sprintf("Auction Player %d", i)),
		)
	}
	return fixture
}

// scriptedRoleRead answers POST /xi/player-roles and nothing else: the first two ids are
// keepers, the next two bowling options, the rest neither. An id it was not told about is
// reported unknown, which is what the served state does for a player it has never seen.
//
// Every other path fails the test on sight. That is the never-XI-picking rule made
// enforceable: an auction request that reached `/xi/optimize` would fail here, in the
// suite that exercises the real handlers, rather than being caught by inspection.
func scriptedRoleRead(t *testing.T, known []string) *MLClient {
	t.Helper()
	keepers := map[string]bool{}
	bowlers := map[string]bool{}
	for i, id := range known {
		switch {
		case i < 2:
			keepers[id] = true
		case i < 4:
			bowlers[id] = true
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/xi/player-roles", r.URL.Path,
			"the auction module calls the role read and nothing else; /xi/optimize is never reached (plan §8.8)")
		var body struct {
			PlayerIDs []string `json:"player_ids"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		players := make([]map[string]any, 0, len(body.PlayerIDs))
		unknown := make([]string, 0)
		for _, id := range body.PlayerIDs {
			roles := make([]string, 0, 2)
			seen := keepers[id] || bowlers[id] || contains(known, id)
			if keepers[id] {
				roles = append(roles, "keeper")
			}
			if bowlers[id] {
				roles = append(roles, "bowling_option")
			}
			if !seen {
				unknown = append(unknown, id)
			}
			players = append(players, map[string]any{"player_id": id, "known": seen, "roles": roles})
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"format":             "T20",
			"players":            players,
			"unknown_player_ids": unknown,
			"served_ratings": map[string]string{
				"run_id": "20260906T083819Z-36689f80", "ratings_through": "2026-09-02",
			},
		}))
	}))
	t.Cleanup(server.Close)
	return &MLClient{BaseURL: server.URL, HTTP: server.Client()}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// callAuction runs one request through the real router and returns the decoded answer.
func callAuction(t *testing.T, router http.Handler, method, path string, body any) (int, auctionResponse) {
	t.Helper()
	recorder := doAuctionRequest(t, router, method, path, body)
	var answer auctionResponse
	if recorder.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
	}
	return recorder.Code, answer
}

func doAuctionRequest(
	t *testing.T,
	router http.Handler,
	method, path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-API-Key", testAPIKey)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// newAuctionOnRecord creates an auction through the handler and lists every fixture player.
func newAuctionOnRecord(t *testing.T, fixture auctionFixture) (http.Handler, auctionResponse) {
	t.Helper()
	app := NewApp(context.Background(), scriptedRoleRead(t, fixture.externalIDs))
	router := NewRouter(app)

	status, created := callAuction(t, router, http.MethodPost, "/api/auctions", map[string]any{
		"name": "IPL 2027", "format": "T20", "buyer_club_id": fixture.buyerID,
		"squad_size": 4, "min_bowlers": 2, "require_keeper": true,
	})
	require.Equal(t, http.StatusOK, status)

	status, listed := callAuction(t, router,
		http.MethodPost, "/api/auctions/"+created.Auction.ID+"/players",
		map[string]any{"player_ids": fixture.playerIDs})
	require.Equal(t, http.StatusOK, status)
	return router, listed
}

func TestAuction_SurvivesAReloadExactlyAsEntered_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	router, listed := newAuctionOnRecord(t, fixture)
	price := int64(1200)

	_, afterSale := callAuction(t, router,
		http.MethodPost, "/api/auctions/"+listed.Auction.ID+"/outcomes",
		map[string]any{
			"player_id": fixture.playerIDs[0], "state": auction.StateSold,
			"buyer_name": "Buying Franchise", "buyer_club_id": fixture.buyerID, "price": price,
		})
	status, reloaded := callAuction(t, router, http.MethodGet, "/api/auctions/"+listed.Auction.ID, nil)

	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, afterSale.Auction, reloaded.Auction,
		"a read after a write returns the record the write returned; nothing is held in the client")
	sold := auctionPlayerByID(t, reloaded, fixture.playerIDs[0])
	assert.Equal(t, auction.StateSold, sold.State)
	assert.Equal(t, "Buying Franchise", sold.BuyerName)
	require.NotNil(t, sold.Price)
	assert.Equal(t, price, *sold.Price)
	assert.Equal(t, 6, len(reloaded.Auction.Players), "every listed player is still on the record")
}

func TestAuction_RecordsEveryStateTransitionIncludingAnUndo_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	router, listed := newAuctionOnRecord(t, fixture)
	price := int64(500)
	outcomes := "/api/auctions/" + listed.Auction.ID + "/outcomes"

	_, _ = callAuction(t, router, http.MethodPost, outcomes, map[string]any{
		"player_id": fixture.playerIDs[1], "state": auction.StateSold,
		"buyer_name": "Rival Franchise", "buyer_club_id": fixture.rivalID, "price": price,
	})
	_, unsold := callAuction(t, router, http.MethodPost, outcomes, map[string]any{
		"player_id": fixture.playerIDs[2], "state": auction.StateUnsold,
	})
	_, undone := callAuction(t, router, http.MethodPost, outcomes, map[string]any{
		"player_id": fixture.playerIDs[1], "state": auction.StateAvailable,
	})

	assert.Equal(t, auction.StateUnsold, auctionPlayerByID(t, unsold, fixture.playerIDs[2]).State)
	back := auctionPlayerByID(t, undone, fixture.playerIDs[1])
	assert.Equal(t, auction.StateAvailable, back.State)
	assert.Empty(t, back.BuyerName, "an undo clears the buyer with the state")
	assert.Nil(t, back.Price, "an undo clears the price with the state")
	assert.Equal(t, 0, undone.Squad.Size, "the rival's purchase was never in this buyer's squad")
}

func TestAuction_SquadAndOpenSlotsFollowTheBuyersOwnPurchases_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	router, listed := newAuctionOnRecord(t, fixture)
	outcomes := "/api/auctions/" + listed.Auction.ID + "/outcomes"
	price := int64(100)

	require.Equal(t, 4, listed.Slots.Open, "nothing bought yet, so every place is open")
	require.NotNil(t, listed.Slots.ByRole)
	require.True(t, listed.Slots.ByRole.KeeperNeeded, "an empty squad needs the keeper the auction requires")

	// The first fixture player is a keeper; the third is a bowling option.
	_, afterKeeper := callAuction(t, router, http.MethodPost, outcomes, map[string]any{
		"player_id": fixture.playerIDs[0], "state": auction.StateSold,
		"buyer_name": "Buying Franchise", "buyer_club_id": fixture.buyerID, "price": price,
	})
	_, afterBowler := callAuction(t, router, http.MethodPost, outcomes, map[string]any{
		"player_id": fixture.playerIDs[2], "state": auction.StateSold,
		"buyer_name": "Buying Franchise", "buyer_club_id": fixture.buyerID, "price": price,
	})

	assert.False(t, afterKeeper.Slots.ByRole.KeeperNeeded, "the squad now holds a keeper")
	assert.Equal(t, 3, afterKeeper.Slots.Open)
	assert.Equal(t, 2, afterBowler.Slots.Open)
	assert.Equal(t, 1, afterBowler.Slots.ByRole.BowlingOptions)
	assert.Equal(t, 1, afterBowler.Slots.ByRole.BowlingOptionsShort,
		"two bowling options were asked for and one is bought")
}

func TestAuction_RoleDistributionMovesOnEverySale_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	router, listed := newAuctionOnRecord(t, fixture)
	outcomes := "/api/auctions/" + listed.Auction.ID + "/outcomes"
	price := int64(100)

	require.NotNil(t, listed.Distribution)
	assert.Equal(t, auctionDistribution{Available: 6, Keepers: 2, BowlingOptions: 2, Batters: 2},
		*listed.Distribution,
		"the roles are the model's two predicates; a player answering neither is a batter by elimination")

	_, afterKeeper := callAuction(t, router, http.MethodPost, outcomes, map[string]any{
		"player_id": fixture.playerIDs[0], "state": auction.StateSold,
		"buyer_name": "Rival Franchise", "buyer_club_id": fixture.rivalID, "price": price,
	})
	_, afterUnsold := callAuction(t, router, http.MethodPost, outcomes, map[string]any{
		"player_id": fixture.playerIDs[5], "state": auction.StateUnsold,
	})

	require.NotNil(t, afterKeeper.Distribution)
	assert.Equal(t, auctionDistribution{Available: 5, Keepers: 1, BowlingOptions: 2, Batters: 2},
		*afterKeeper.Distribution, "selling a keeper takes him out of the remaining pool")
	require.NotNil(t, afterUnsold.Distribution)
	assert.Equal(t, auctionDistribution{Available: 4, Keepers: 1, BowlingOptions: 2, Batters: 1},
		*afterUnsold.Distribution, "an unsold player is off the available pool too")
}

func TestAuction_RefusesASaleWithNoPrice_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	router, listed := newAuctionOnRecord(t, fixture)

	recorder := doAuctionRequest(t, router,
		http.MethodPost, "/api/auctions/"+listed.Auction.ID+"/outcomes",
		map[string]any{
			"player_id": fixture.playerIDs[0], "state": auction.StateSold, "buyer_name": "Rival",
		})

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "a sale names the price")
}

func TestAuction_RefusesAnOutcomeForAPlayerNobodyListed_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	router, listed := newAuctionOnRecord(t, fixture)

	recorder := doAuctionRequest(t, router,
		http.MethodPost, "/api/auctions/"+listed.Auction.ID+"/outcomes",
		map[string]any{"player_id": 999999, "state": auction.StateUnsold})

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "AUCTION_NOT_FOUND")
}

func TestAuction_ListingAPlayerTwiceKeepsTheStateHeIsIn_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	router, listed := newAuctionOnRecord(t, fixture)
	price := int64(300)

	_, _ = callAuction(t, router, http.MethodPost, "/api/auctions/"+listed.Auction.ID+"/outcomes",
		map[string]any{
			"player_id": fixture.playerIDs[3], "state": auction.StateSold,
			"buyer_name": "Rival Franchise", "buyer_club_id": fixture.rivalID, "price": price,
		})
	_, again := callAuction(t, router,
		http.MethodPost, "/api/auctions/"+listed.Auction.ID+"/players",
		map[string]any{"player_ids": fixture.playerIDs})

	assert.Equal(t, auction.StateSold, auctionPlayerByID(t, again, fixture.playerIDs[3]).State,
		"re-adding a sold player is a double-click, not an instruction to forget the sale")
	assert.Len(t, again.Auction.Players, 6, "and it lists nobody twice")
}

func TestSearchPlayers_FindsAPlayerAcrossClubsByNamePrefix_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	fixture := seedAuctionFixture(t)
	app := NewApp(context.Background(), scriptedRoleRead(t, fixture.externalIDs))
	router := NewRouter(app)

	recorder := doAuctionRequest(t, router, http.MethodGet, "/api/players/search?q=Auction", nil)

	require.Equal(t, http.StatusOK, recorder.Code)
	var answer playerSearchResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
	assert.Len(t, answer.Players, 6)
	assert.Equal(t, "Auction Player 0", answer.Players[0].PlayerName)
	assert.False(t, answer.Players[0].IsWicketKeeper,
		"this is the database's name-set flag and not the model's role, and it is named as such")
}

func TestSearchPlayers_RefusesAPrefixTooShortToMeanAnything_Integration(t *testing.T) {
	dbtest.SkipUnlessScratchDatabase(t)
	seedAuctionFixture(t)
	router := NewRouter(NewApp(context.Background(), &MLClient{}))

	recorder := doAuctionRequest(t, router, http.MethodGet, "/api/players/search?q=a", nil)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "at least two characters")
}

// auctionPlayerByID finds one listed player on an answer, failing the test where the
// record does not hold him.
func auctionPlayerByID(t *testing.T, answer auctionResponse, playerID int64) auctionPlayer {
	t.Helper()
	for _, player := range answer.Auction.Players {
		if player.PlayerID == playerID {
			return player
		}
	}
	require.FailNowf(t, "player not on the record", "player %d", playerID)
	return auctionPlayer{}
}
