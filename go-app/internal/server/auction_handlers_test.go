package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
	"github.com/umayangag/cric-flow/go-app/internal/auction/mocks"
)

// The auction handlers against a mock store and a scripted ml-service (P3-1).
//
// The integration suite pins the SQL; these pin the wire. What matters here is the
// answer's shape in every state the role read can be in — served, refused by a stale
// registry, refused by an unreachable service — and the rule that no request the module
// makes reaches `/xi/optimize`.

// storedAuction is the record a mock store answers with: four listed players, one bought
// by the auction's own side and one by a rival.
func storedAuction() auction.Auction {
	price := int64(900)
	return auction.Auction{
		ID:                "auction-1",
		CreatedAt:         time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC),
		Name:              "IPL 2027",
		FormatCode:        "T20",
		BuyerOppositionID: 11,
		BuyerName:         "Buying Franchise",
		VenueIDs:          []int64{4, 9},
		SquadSize:         3,
		MinBowlers:        2,
		RequireKeeper:     true,
		Players: []auction.ListedPlayer{
			{
				PlayerID: 1, ExternalID: "aaa1", PlayerName: "Keeper Sold", State: auction.StateSold,
				BuyerName: "Buying Franchise", BuyerOppositionID: 11, Price: &price,
				StateChangedAt: time.Date(2026, 9, 8, 11, 0, 0, 0, time.UTC),
			},
			{
				PlayerID: 2, ExternalID: "aaa2", PlayerName: "Bowler Available",
				State:          auction.StateAvailable,
				StateChangedAt: time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC),
			},
			{
				PlayerID: 3, ExternalID: "aaa3", PlayerName: "Batter Available",
				State:          auction.StateAvailable,
				StateChangedAt: time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC),
			},
			{
				PlayerID: 4, PlayerName: "No Registry Id", State: auction.StateAvailable,
				StateChangedAt: time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC),
			},
		},
	}
}

// roleReadServer answers the role read, and records every path it was asked for.
func roleReadServer(t *testing.T, paths *[]string) *MLClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		var body struct {
			PlayerIDs []string `json:"player_ids"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		roles := map[string][]string{"aaa1": {"keeper"}, "aaa2": {"bowling_option"}, "aaa3": {}}
		players := make([]map[string]any, 0, len(body.PlayerIDs))
		for _, id := range body.PlayerIDs {
			read, known := roles[id]
			players = append(players, map[string]any{
				"player_id": id, "known": known, "roles": read,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"format": "T20", "players": players, "unknown_player_ids": []string{},
			"served_ratings": map[string]string{
				"run_id": "20260906T083819Z-36689f80", "ratings_through": "2026-09-02",
			},
		}))
	}))
	t.Cleanup(server.Close)
	return &MLClient{BaseURL: server.URL, HTTP: server.Client()}
}

// refusingMLService answers every call with one refusal, the way ml-service writes one.
func refusingMLService(t *testing.T, status int, code, message string) *MLClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"detail": map[string]any{"code": code, "message": message, "hint": "retrain, then reload"},
		}))
	}))
	t.Cleanup(server.Close)
	return &MLClient{BaseURL: server.URL, HTTP: server.Client()}
}

// getAuctionThrough runs GET /api/auctions/{id} against the app, with the path variable
// set the way the router would.
func getAuctionThrough(t *testing.T, app *App) *httptest.ResponseRecorder {
	t.Helper()
	request := mux.SetURLVars(
		httptest.NewRequest(http.MethodGet, "/api/auctions/auction-1", nil),
		map[string]string{"id": "auction-1"},
	)
	recorder := httptest.NewRecorder()
	app.getAuctionHandler(recorder, request)
	return recorder
}

func decodeAuction(t *testing.T, recorder *httptest.ResponseRecorder) auctionResponse {
	t.Helper()
	var answer auctionResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
	return answer
}

func TestGetAuctionHandler_CarriesTheRolesTheRunAndTheDateBesideTheRecord(t *testing.T) {
	store := mocks.NewMockStore(t)
	store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(storedAuction()), nil)
	var paths []string
	app := &App{auctionStore: store, mlClient: roleReadServer(t, &paths)}

	recorder := getAuctionThrough(t, app)

	require.Equal(t, http.StatusOK, recorder.Code)
	answer := decodeAuction(t, recorder)
	assert.True(t, answer.Roles.Available)
	assert.Equal(t, "20260906T083819Z-36689f80", answer.Roles.RunID)
	assert.Equal(t, "2026-09-02", answer.Roles.RatingsThrough,
		"a count off the served vectors carries the run and date it was read from (P1-5)")
	require.NotNil(t, answer.Auction.Players[0].Roles)
	assert.Equal(t, []string{"keeper"}, answer.Auction.Players[0].Roles.Roles)
	assert.Equal(t, []string{"bowling_option"}, answer.Auction.Players[1].Roles.Roles)
	assert.Empty(t, answer.Auction.Players[2].Roles.Roles,
		"a known player answering neither predicate is a batter by elimination")
}

func TestGetAuctionHandler_ReportsAPlayerTheServedStateHasNotSeenAsUnknown(t *testing.T) {
	store := mocks.NewMockStore(t)
	store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(storedAuction()), nil)
	var paths []string
	app := &App{auctionStore: store, mlClient: roleReadServer(t, &paths)}

	answer := decodeAuction(t, getAuctionThrough(t, app))

	unknown := answer.Auction.Players[3]
	require.NotNil(t, unknown.Roles)
	assert.False(t, unknown.Roles.Known)
	assert.Empty(t, unknown.Roles.Roles, "no role is invented for a player the model has never read (§8.7)")
	require.NotNil(t, answer.Distribution)
	assert.Equal(t, 1, answer.Distribution.Unknown)
	assert.Equal(t, 1, answer.Distribution.Batters,
		"unknown is counted apart from the batters; the two are different answers")
}

func TestGetAuctionHandler_CountsTheRemainingPoolAndTheOpenSlots(t *testing.T) {
	store := mocks.NewMockStore(t)
	store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(storedAuction()), nil)
	var paths []string
	app := &App{auctionStore: store, mlClient: roleReadServer(t, &paths)}

	answer := decodeAuction(t, getAuctionThrough(t, app))

	assert.Equal(t, 1, answer.Squad.Size, "the rival's purchases are not this buyer's squad")
	assert.Equal(t, 2, answer.Slots.Open)
	require.NotNil(t, answer.Slots.ByRole)
	assert.False(t, answer.Slots.ByRole.KeeperNeeded)
	assert.Equal(t, 2, answer.Slots.ByRole.BowlingOptionsShort)
	require.NotNil(t, answer.Distribution)
	assert.Equal(t, 3, answer.Distribution.Available)
}

func TestGetAuctionHandler_OnAStaleRegistryShowsTheListAndNamesTheRefusal(t *testing.T) {
	store := mocks.NewMockStore(t)
	store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(storedAuction()), nil)
	app := &App{
		auctionStore: store,
		mlClient: refusingMLService(t, http.StatusServiceUnavailable, "RATINGS_STALE",
			"ratings run through 2026-07-01 (69 days old, limit 14)"),
	}

	recorder := getAuctionThrough(t, app)

	require.Equal(t, http.StatusOK, recorder.Code,
		"the record is facts the operator typed; reading it back needs no model")
	answer := decodeAuction(t, recorder)
	assert.False(t, answer.Roles.Available)
	assert.Equal(t, "RATINGS_STALE", answer.Roles.Code, "the refusal is on the wire by name, not only in a log")
	assert.Contains(t, answer.Roles.Message, "69 days old")
	assert.Len(t, answer.Auction.Players, 4, "the list is still shown")
	assert.Nil(t, answer.Distribution, "and no count is shown off a model that refused to answer")
	assert.Nil(t, answer.Slots.ByRole)
	assert.Equal(t, 2, answer.Slots.Open, "the places left are arithmetic over the record alone")
	assert.Nil(t, answer.Auction.Players[0].Roles, "absent, not empty: the model was not asked")
}

func TestGetAuctionHandler_WhenMLServiceIsUnreachableNamesThatRefusalToo(t *testing.T) {
	store := mocks.NewMockStore(t)
	store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(storedAuction()), nil)
	app := &App{auctionStore: store, mlClient: &MLClient{
		BaseURL: "http://127.0.0.1:1", HTTP: &http.Client{Timeout: time.Second},
	}}

	answer := decodeAuction(t, getAuctionThrough(t, app))

	assert.False(t, answer.Roles.Available)
	assert.Equal(t, mlUnreachableCode, answer.Roles.Code)
	assert.Len(t, answer.Auction.Players, 4)
}

func TestGetAuctionHandler_AnAuctionNobodyCreatedIsA404(t *testing.T) {
	store := mocks.NewMockStore(t)
	store.EXPECT().Get(mock.Anything, "auction-1").Return(nil, auction.ErrNotFound)
	app := &App{auctionStore: store, mlClient: &MLClient{}}

	recorder := getAuctionThrough(t, app)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "AUCTION_NOT_FOUND")
}

func TestAuctionEndpoints_NeverReachXiOptimize(t *testing.T) {
	store := mocks.NewMockStore(t)
	record := storedAuction()
	store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(record), nil)
	store.EXPECT().Create(mock.Anything, mock.Anything).Return(ptr(record), nil)
	store.EXPECT().AddPlayers(mock.Anything, "auction-1", []int64{2}).Return(ptr(record), nil)
	store.EXPECT().RecordOutcome(mock.Anything, "auction-1", mock.Anything).Return(ptr(record), nil)
	var paths []string
	app := &App{auctionStore: store, mlClient: roleReadServer(t, &paths)}
	price := int64(400)

	callAuctionHandler(t, app.createAuctionHandler, http.MethodPost, "/api/auctions", nil,
		createAuctionRequest{Name: "IPL", Format: "T20", BuyerClubID: 11, SquadSize: 3})
	getAuctionThrough(t, app)
	callAuctionHandler(t, app.addAuctionPlayersHandler,
		http.MethodPost, "/api/auctions/auction-1/players", map[string]string{"id": "auction-1"},
		addAuctionPlayersRequest{PlayerIDs: []int64{2}})
	callAuctionHandler(t, app.recordAuctionOutcomeHandler,
		http.MethodPost, "/api/auctions/auction-1/outcomes", map[string]string{"id": "auction-1"},
		recordOutcomeRequest{
			PlayerID: 2, State: auction.StateSold, BuyerName: "Rival", BuyerClubID: 12, Price: &price,
		})

	require.NotEmpty(t, paths, "every one of these endpoints reads the roles")
	for _, path := range paths {
		assert.Equal(t, "/xi/player-roles", path,
			"this module is valuation and never XI-picking: in T20 the system has not shown it can "+
				"choose an eleven better than rating order (plan §8.8), so nothing here calls /xi/optimize")
	}
}

func TestCreateAuctionHandler_RefusesAFormatTheSimulatorDoesNotServe(t *testing.T) {
	app := &App{auctionStore: mocks.NewMockStore(t), mlClient: &MLClient{}}

	recorder := callAuctionHandler(t, app.createAuctionHandler, http.MethodPost, "/api/auctions", nil,
		createAuctionRequest{Name: "The Ashes", Format: "TEST", BuyerClubID: 11, SquadSize: 3})

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "innings length",
		"every projection this module makes needs one, so a format without one is refused at creation")
}

func TestCreateAuctionHandler_RefusesTheSetupsThatAreNotAnAuction(t *testing.T) {
	testCases := []struct {
		name    string
		request createAuctionRequest
		wantMsg string
	}{
		{
			name:    "no name",
			request: createAuctionRequest{Format: "T20", BuyerClubID: 11, SquadSize: 3},
			wantMsg: "an auction needs a name",
		},
		{
			name:    "no buying side",
			request: createAuctionRequest{Name: "IPL", Format: "T20", SquadSize: 3},
			wantMsg: "buyer_club_id is required",
		},
		{
			name:    "no squad to fill",
			request: createAuctionRequest{Name: "IPL", Format: "T20", BuyerClubID: 11},
			wantMsg: "squad_size must be",
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			app := &App{auctionStore: mocks.NewMockStore(t), mlClient: &MLClient{}}

			recorder := callAuctionHandler(t, app.createAuctionHandler,
				http.MethodPost, "/api/auctions", nil, testCase.request)

			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Contains(t, recorder.Body.String(), testCase.wantMsg)
		})
	}
}

func TestCreateAuctionHandler_DefaultsTheConstraintsToThePredictPaths(t *testing.T) {
	store := mocks.NewMockStore(t)
	var created auction.Auction
	store.EXPECT().Create(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, record auction.Auction) (*auction.Auction, error) {
			created = record
			return ptr(storedAuction()), nil
		})
	var paths []string
	app := &App{auctionStore: store, mlClient: roleReadServer(t, &paths)}

	callAuctionHandler(t, app.createAuctionHandler, http.MethodPost, "/api/auctions", nil,
		createAuctionRequest{Name: "IPL", Format: "t20", BuyerClubID: 11, SquadSize: 25})

	assert.Equal(t, "T20", created.FormatCode, "a format is normalised the way every other path normalises one")
	assert.Equal(t, auctionDefaultMinBowlers, created.MinBowlers)
	assert.True(t, created.RequireKeeper,
		"an auction's constraints are the eleven's, and the eleven's defaults are the predict path's")
	assert.NotEmpty(t, created.ID)
}

func TestRecordOutcomeHandler_RefusesAnEntryThatIsNotAStateOfAnAuction(t *testing.T) {
	app := &App{auctionStore: mocks.NewMockStore(t), mlClient: &MLClient{}}

	recorder := callAuctionHandler(t, app.recordAuctionOutcomeHandler,
		http.MethodPost, "/api/auctions/auction-1/outcomes", map[string]string{"id": "auction-1"},
		recordOutcomeRequest{PlayerID: 2, State: "withdrawn"})

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "available", "the answer names the states there are")
}

func TestListAuctionsHandler_IsTheIndexAnOperatorFindsAnAuctionFromAfterAReload(t *testing.T) {
	store := mocks.NewMockStore(t)
	store.EXPECT().List(mock.Anything).Return([]auction.Auction{storedAuction()}, nil)
	app := &App{auctionStore: store, mlClient: &MLClient{}}

	recorder := callAuctionHandler(t, app.listAuctionsHandler, http.MethodGet, "/api/auctions", nil, nil)

	require.Equal(t, http.StatusOK, recorder.Code)
	var answer struct {
		Auctions []auctionSummary `json:"auctions"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &answer))
	require.Len(t, answer.Auctions, 1)
	assert.Equal(t, "IPL 2027", answer.Auctions[0].Name)
	assert.Equal(t, "Buying Franchise", answer.Auctions[0].Buyer.Name)
}

func TestListAuctionsHandler_AStoreFailureIsReportedAndNotSwallowed(t *testing.T) {
	store := mocks.NewMockStore(t)
	store.EXPECT().List(mock.Anything).Return(nil, errors.New("connection refused"))
	app := &App{auctionStore: store, mlClient: &MLClient{}}

	recorder := callAuctionHandler(t, app.listAuctionsHandler, http.MethodGet, "/api/auctions", nil, nil)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "connection refused")
}

func TestGetAuctionHandler_AnEmptyListIsReportedAsNotReadRatherThanAsZeroKeepers(t *testing.T) {
	store := mocks.NewMockStore(t)
	empty := storedAuction()
	empty.Players = nil
	store.EXPECT().Get(mock.Anything, "auction-1").Return(ptr(empty), nil)
	// No ml-service behind the client at all: an empty list must produce no request, so a
	// base URL nothing answers on is the strongest way to say the read did not happen.
	app := &App{auctionStore: store, mlClient: &MLClient{BaseURL: "http://127.0.0.1:1"}}

	answer := decodeAuction(t, getAuctionThrough(t, app))

	assert.False(t, answer.Roles.Available)
	assert.Equal(t, "ROLES_NOT_READ", answer.Roles.Code)
	assert.Nil(t, answer.Distribution,
		"an all-zero distribution would look exactly like a served answer with no keepers left")
	assert.Nil(t, answer.Slots.ByRole)
	assert.Equal(t, 3, answer.Slots.Open, "the places left need no model")
}

// callAuctionHandler runs one handler with a JSON body and the path variables the router
// would have set.
func callAuctionHandler(
	t *testing.T,
	handler http.HandlerFunc,
	method, path string,
	vars map[string]string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader([]byte("{}"))
	} else {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, reader)
	if vars != nil {
		request = mux.SetURLVars(request, vars)
	}
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	return recorder
}

// ptr returns a pointer to a value, which is what a store returns and a literal is not.
func ptr[T any](value T) *T { return &value }
