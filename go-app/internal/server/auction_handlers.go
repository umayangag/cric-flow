package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/umayangag/cric-flow/go-app/internal/auction"
	"github.com/umayangag/cric-flow/go-app/internal/availability"
	"github.com/umayangag/cric-flow/go-app/internal/config"
	"github.com/umayangag/cric-flow/go-app/internal/db"
	"github.com/umayangag/cric-flow/go-app/internal/services/predictteam"
)

// The auction endpoints (P3-1): create an auction, list players onto it, record what the
// room did, and read it whole.
//
// The module is valuation and projection and never XI-picking. The record is that
// optimised selection in domestic T20 is indistinguishable from rating order (plan §8.8),
// and the IPL is domestic T20 — so nothing here calls `/xi/optimize`, nothing here
// computes a marginal value and nothing here returns a win probability. The one
// ml-service call any of these handlers makes is the role read, and a test asserts that
// through the client.

// auctionStore is the record's persistence. Nil is the process default — the
// database-backed store — so a handler built without one still works; tests set a mock.
func (a *App) auctionRecord() auction.Store {
	if a != nil && a.auctionStore != nil {
		return a.auctionStore
	}
	return db.NewAuctionStore()
}

// createAuctionRequest is what an operator sets up before the room opens.
type createAuctionRequest struct {
	Name          string  `json:"name"`
	Format        string  `json:"format"`
	BuyerClubID   int64   `json:"buyer_club_id"`
	VenueIDs      []int64 `json:"venue_ids"`
	SquadSize     int     `json:"squad_size"`
	MinBowlers    *int    `json:"min_bowlers"`
	RequireKeeper *bool   `json:"require_keeper"`
}

// The constraint defaults, which are the predict path's: an eleven with a keeper and five
// bowling options. They are defaults and not fixtures — a squad is assembled under the
// constraints the operator says the eleven has — but they are the same two values
// `predictteam` starts from, so an auction and a prediction mean the same thing by "five
// bowlers".
const (
	auctionDefaultMinBowlers    = 5
	auctionDefaultRequireKeeper = true
	auctionSquadSizeMax         = 60
)

// createAuctionHandler answers POST /api/auctions.
func (a *App) createAuctionHandler(w http.ResponseWriter, r *http.Request) {
	var request createAuctionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_BODY",
			Message: "the request body is not valid JSON: " + err.Error(),
		})
		return
	}
	record, apiErr := newAuctionFromRequest(request)
	if apiErr != nil {
		writeJSON(w, http.StatusBadRequest, *apiErr)
		return
	}
	created, err := a.auctionRecord().Create(r.Context(), *record)
	if err != nil {
		slog.Error("createAuction: writing the record failed", slog.Any("err", err))
		respondErr(w, err)
		return
	}
	a.respondWithAuction(w, r, created)
}

// newAuctionFromRequest validates what the operator set up and mints the record's id.
//
// The format must be one the simulator serves. Every later item of Phase 3 projects a
// total, and a total needs an innings length; an auction for TEST would be a record whose
// whole point could never be computed, and refusing it here is better than discovering it
// four items later.
func newAuctionFromRequest(request createAuctionRequest) (*auction.Auction, *apiError) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return nil, &apiError{Code: "INVALID_PARAM", Message: "an auction needs a name"}
	}
	format := predictteam.NormalizeFormat(request.Format)
	if !predictteam.FormatHasInningsLength(format) {
		return nil, &apiError{
			Code:      "INVALID_PARAM",
			Message:   "format must be one the simulator serves; every projection this module makes needs an innings length",
			Available: predictteam.SimulatedFormatCodes(),
		}
	}
	if request.BuyerClubID <= 0 {
		return nil, &apiError{
			Code:    "INVALID_PARAM",
			Message: "buyer_club_id is required: an auction fills a squad for one side",
			Hint:    "buyer_club_id is the club_id from /api/options/teams-by-format",
		}
	}
	if request.SquadSize <= 0 || request.SquadSize > auctionSquadSizeMax {
		return nil, &apiError{
			Code:    "INVALID_PARAM",
			Message: "squad_size must be a whole number of places between 1 and " + strconv.Itoa(auctionSquadSizeMax),
		}
	}
	minBowlers := auctionDefaultMinBowlers
	if request.MinBowlers != nil {
		minBowlers = *request.MinBowlers
	}
	if minBowlers < 0 || minBowlers > 11 {
		return nil, &apiError{
			Code:    "INVALID_PARAM",
			Message: "min_bowlers is the eleven's constraint, so it is between 0 and 11",
		}
	}
	requireKeeper := auctionDefaultRequireKeeper
	if request.RequireKeeper != nil {
		requireKeeper = *request.RequireKeeper
	}
	venueIDs := request.VenueIDs
	if venueIDs == nil {
		venueIDs = []int64{}
	}
	return &auction.Auction{
		ID:                auction.NewID(),
		Name:              name,
		FormatCode:        format,
		BuyerOppositionID: request.BuyerClubID,
		VenueIDs:          venueIDs,
		SquadSize:         request.SquadSize,
		MinBowlers:        minBowlers,
		RequireKeeper:     requireKeeper,
	}, nil
}

// listAuctionsHandler answers GET /api/auctions: the index an operator finds an auction
// from after a reload.
func (a *App) listAuctionsHandler(w http.ResponseWriter, r *http.Request) {
	auctions, err := a.auctionRecord().List(r.Context())
	if err != nil {
		slog.Error("listAuctions: reading the record failed", slog.Any("err", err))
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"auctions": newAuctionSummaries(auctions)})
}

// getAuctionHandler answers GET /api/auctions/{id}: the record whole, with the roles.
func (a *App) getAuctionHandler(w http.ResponseWriter, r *http.Request) {
	record, ok := a.loadAuction(w, r)
	if !ok {
		return
	}
	a.respondWithAuction(w, r, record)
}

// addAuctionPlayersRequest lists players onto an auction, by id.
type addAuctionPlayersRequest struct {
	PlayerIDs []int64 `json:"player_ids"`
}

// addAuctionPlayersHandler answers POST /api/auctions/{id}/players.
func (a *App) addAuctionPlayersHandler(w http.ResponseWriter, r *http.Request) {
	auctionID := strings.TrimSpace(mux.Vars(r)["id"])
	var request addAuctionPlayersRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_BODY",
			Message: "the request body is not valid JSON: " + err.Error(),
		})
		return
	}
	if len(request.PlayerIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_PARAM",
			Message: "player_ids names at least one player to list",
			Hint:    "ids come from GET /api/players/search",
		})
		return
	}
	record, err := a.auctionRecord().AddPlayers(r.Context(), auctionID, request.PlayerIDs)
	if err != nil {
		a.respondAuctionErr(w, auctionID, "addAuctionPlayers", err)
		return
	}
	a.respondWithAuction(w, r, record)
}

// recordOutcomeRequest is one entry the operator makes as the room moves on.
type recordOutcomeRequest struct {
	PlayerID    int64  `json:"player_id"`
	State       string `json:"state"`
	BuyerName   string `json:"buyer_name"`
	BuyerClubID int64  `json:"buyer_club_id"`
	Price       *int64 `json:"price"`
}

// recordAuctionOutcomeHandler answers POST /api/auctions/{id}/outcomes: a sale, an unsold
// result, or an undo back to available.
//
// The undo is not a separate endpoint because it is not a separate kind of fact: the
// operator mistyped, and the record now says the player is available again — with the
// buyer and the price cleared, because a player back on the list who still carries what
// somebody paid for him is a record of two contradictory things.
func (a *App) recordAuctionOutcomeHandler(w http.ResponseWriter, r *http.Request) {
	auctionID := strings.TrimSpace(mux.Vars(r)["id"])
	var request recordOutcomeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_BODY",
			Message: "the request body is not valid JSON: " + err.Error(),
		})
		return
	}
	outcome := auction.Outcome{
		PlayerID:          request.PlayerID,
		State:             strings.TrimSpace(strings.ToLower(request.State)),
		BuyerName:         strings.TrimSpace(request.BuyerName),
		BuyerOppositionID: request.BuyerClubID,
		Price:             request.Price,
	}
	if err := outcome.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:      "INVALID_PARAM",
			Message:   err.Error(),
			Available: auction.States(),
		})
		return
	}
	record, err := a.auctionRecord().RecordOutcome(r.Context(), auctionID, outcome)
	if err != nil {
		a.respondAuctionErr(w, auctionID, "recordAuctionOutcome", err)
		return
	}
	a.respondWithAuction(w, r, record)
}

// loadAuction reads the record named in the path, answering 404 where there is none.
func (a *App) loadAuction(w http.ResponseWriter, r *http.Request) (*auction.Auction, bool) {
	auctionID := strings.TrimSpace(mux.Vars(r)["id"])
	record, err := a.auctionRecord().Get(r.Context(), auctionID)
	if err != nil {
		a.respondAuctionErr(w, auctionID, "getAuction", err)
		return nil, false
	}
	return record, true
}

// respondAuctionErr answers a store failure, naming a missing auction as one.
//
// `ErrNotFound` covers both a missing auction and a player the list does not hold; the
// message says so rather than leaving an operator who mistyped a player id to conclude
// their auction has vanished.
func (a *App) respondAuctionErr(w http.ResponseWriter, auctionID, operation string, err error) {
	if errors.Is(err, auction.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, apiError{
			Code:    "AUCTION_NOT_FOUND",
			Message: "no auction " + auctionID + ", or no such player on its list",
			Hint:    "auction ids come from GET /api/auctions; a player must be listed before an outcome is recorded for him",
		})
		return
	}
	slog.Error(operation+": the auction record failed",
		slog.String("auction_id", auctionID), slog.Any("err", err))
	respondErr(w, err)
}

// searchPlayersHandler answers GET /api/players/search: a cross-club name search.
//
// `GET /api/options/candidates` is per club because a prediction is about one side; an
// auction room is not one side, so this is its own read. It applies no recency window and
// removes nobody: the ledger's verdict is shown beside a name, because what the room is
// selling is not a pool this system gets to filter.
func (a *App) searchPlayersHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	prefix := strings.TrimSpace(query.Get("q"))
	if len(prefix) < 2 {
		writeJSON(w, http.StatusBadRequest, apiError{
			Code:    "INVALID_PARAM",
			Message: "q is the start of a player's name, and needs at least two characters",
		})
		return
	}
	limit, apiErr := parsePositiveParam(query.Get("limit"), "limit")
	if apiErr != nil {
		writeJSON(w, http.StatusBadRequest, *apiErr)
		return
	}
	flags, err := availability.NewLedger(db.NewPlayerStatusStore(), availability.Criteria(config.Load())).
		Flags(r.Context(), actorFrom(r))
	if err != nil {
		slog.Error("searchPlayers: reading the retirement ledger failed", slog.Any("err", err))
		respondErr(w, err)
		return
	}
	found, err := db.SearchPlayers(r.Context(), db.PlayerSearchQuery{
		Prefix:     prefix,
		FormatCode: predictteam.NormalizeFormat(query.Get("format")),
		Limit:      limit,
		Flags:      flags,
	})
	if err != nil {
		slog.Error("searchPlayers: the search failed", slog.String("q", prefix), slog.Any("err", err))
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, playerSearchResponse{Players: newPlayerSearchResults(found)})
}
